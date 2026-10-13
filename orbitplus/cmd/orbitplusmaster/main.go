// orbitplusmaster runs the HTTP service that receives and serves TripDetails.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/bits"
	"orbitplusmaster/internal/infrastructure/cassandra"
	"orbitplusmaster/internal/infrastructure/dragonfly"
	masterhttp "orbitplusmaster/internal/infrastructure/http"
	"orbitplusmaster/internal/infrastructure/rabbitmq"
)

// masterServices groups what startup builds. A struct rather than a positional
// return, because the previous signature declared seven results while its own
// return statements supplied eight, and nothing caught it until the package
// failed to compile.
type masterServices struct {
	tripDetails *master.TripDetailsService
	read        *master.TripDetailsReadService
	cache       *master.CacheReadService
	// persistence is the write path, reused as the cache repairer so a repaired
	// document is split and indexed exactly as a Worker refresh would do it.
	persistence *master.TripDetailsStorage
	metadata    *cassandra.TripDetailsMetadataRepository
	metrix      *cassandra.QueueMetrixRepository
	// differences is the busmap_data_analytics store, shared as the live-path
	// difference writer and the report reader. Nil when Cassandra could not be
	// reached for it, which disables recording and the report but not reads.
	differences *cassandra.CacheFreshnessDifferenceRepository
	rateLimiter master.ZoneRateLimiter
	close       func()
}

func main() {
	startedAt := time.Now().UTC()
	log.Print("beginning orbitplusmaster startup")
	config, err := master.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("invalid orbitplusmaster configuration: %v", err)
	}
	log.Printf("orbitplusmaster configuration loaded: APP_ENV=%s MASTER_API_PORT=%d", config.AppEnvironment, config.APIPort)

	closeLogFile, err := setupFileLogging(config.LogDir)
	if err != nil {
		log.Printf("file logging disabled: %v", err)
	} else {
		defer closeLogFile()
		log.Printf("file logging enabled: writing to %s", config.LogDir)
	}

	services, err := newMasterServices(config)
	if err != nil {
		log.Fatalf("initialize TripDetails persistence: %v", err)
	}
	defer services.close()

	var inventoryPublisher master.InventoryEventPublisher
	closeInventoryPublisher := func() {}
	if config.Queue != nil {
		publisher, err := rabbitmq.NewInventoryEventPublisher(config.Queue.URL, config.Queue.Exchange)
		if err != nil {
			log.Fatalf("initialize RabbitMQ inventory event publisher: %v", err)
		}
		inventoryPublisher = publisher
		closeInventoryPublisher = func() { _ = publisher.Close() }
	}
	defer closeInventoryPublisher()

	var rabbitMQManagementReader rabbitmq.ManagementReader
	if config.RabbitMQManagement != nil {
		rabbitMQManagementReader = rabbitmq.NewManagementClient(*config.RabbitMQManagement)
	}

	var operatorRegistry master.OperatorRegistry
	if services.metrix != nil {
		operatorRegistry = services.metrix
	}
	orionmaxInventoryChangeService := master.NewOrionmaxInventoryEventService(inventoryPublisher, services.metadata, services.metrix, operatorRegistry)

	closeOrbitRouteRefresh := func() {}
	switch {
	case config.OrbitRouteRefresh == nil:
		log.Print("Orbit periodic route refresh scheduler disabled: configure ORBIT_ROUTE_BASE_URL, ORBIT_ROUTE_ACCESS_TOKEN, ORBIT_ROUTE_TIMEOUT, ORBIT_ROUTE_REFRESH_INTERVAL, and ORBIT_ROUTE_STALE_DURATION")
	case services.metadata == nil || services.metrix == nil:
		log.Print("Orbit periodic route refresh scheduler disabled: Cassandra persistence and Queue Metrics storage are required")
	case operatorRegistry == nil:
		log.Print("Orbit periodic route refresh scheduler disabled: operator registry is unavailable")
	case inventoryPublisher == nil:
		log.Print("Orbit periodic route refresh scheduler disabled: RabbitMQ publishing is not configured")
	default:
		publisher, ok := inventoryPublisher.(master.PriorityInventoryEventPublisher)
		if !ok {
			log.Print("Orbit periodic route refresh scheduler disabled: RabbitMQ publisher does not support priorities")
		} else {
			closeOrbitRouteRefresh = master.NewOrbitRouteRefreshService(*config.OrbitRouteRefresh, services.metadata, services.metrix, services.metrix, operatorRegistry, publisher).Start()
			log.Print("Orbit periodic route refresh scheduler enabled")
		}
	}
	defer closeOrbitRouteRefresh()

	queueJobsService := master.NewQueueJobsService(services.metrix)
	tripFreshnessService := master.NewTripFreshnessService(services.metadata)
	var tripHistoryReader master.TripHistoryQueueReader
	if services.metrix != nil {
		tripHistoryReader = services.metrix
	}
	tripHistoryService := master.NewTripHistoryService(tripHistoryReader)
	tablesService := master.NewTablesService(services.metadata)
	uiAccessAuth := masterhttp.NewUIAccessAuth(config.UIAccessToken, config.AppEnvironment == master.Production)

	// The difference store is a nil interface unless the repository connected,
	// so the verifier still serves live reads with recording simply disabled.
	var differenceWriter master.CacheDifferenceWriter
	var busmapAnalyticsReader master.BusmapAnalyticsReader
	if services.differences != nil {
		differenceWriter = services.differences
		busmapAnalyticsReader = services.differences
	}
	busmapAnalyticsService := master.NewBusmapAnalyticsService(busmapAnalyticsReader)
	logsService := master.NewLogsReadService(
		master.LogSource{Label: "master", Dir: config.LogDir},
		master.LogSource{Label: "worker", Dir: config.WorkerLogDir},
	)

	freshnessVerifier := newCacheFreshnessVerifier(config, services.read, services.persistence, differenceWriter, services.rateLimiter)

	// Age-based cache freshness: a cacheFlag=1 read that finds Cold data (older
	// than CACHE_WARM_MAX_AGE) refreshes it from BITS. It needs the live verifier
	// (to fetch + repair) and the operator registry (to resolve operator -> zone
	// -> BITS URL). When either is unavailable, the refresher is not attached and
	// reads keep serving whatever cache exists.
	if services.read != nil && freshnessVerifier != nil && operatorRegistry != nil {
		coldRefresher := master.NewColdCacheRefresher(freshnessVerifier, operatorRegistry, log.Default())
		services.read.WithColdRefresher(coldRefresher)
		log.Print("cold cache refresh enabled: Cold reads refresh from BITS via operator zone")
	} else {
		log.Print("cold cache refresh disabled: live verification and operator registry are required")
	}

	// Dedicated log files for Search and BusMap calls so each call type can be
	// inspected on its own.
	searchLogger, closeSearchLog := newNamedFileLogger(config.LogDir, "search")
	defer closeSearchLog()
	busmapLogger, closeBusmapLog := newNamedFileLogger(config.LogDir, "busmap")
	defer closeBusmapLog()
	// Dedicated log file for Orionmax inventory-change events pushed to the
	// master, so inventory pushes can be inspected on their own.
	inventoryLogger, closeInventoryLog := newNamedFileLogger(config.LogDir, "inventory")
	defer closeInventoryLog()
	orionmaxInventoryChangeService.WithLogger(inventoryLogger)

	router := masterhttp.NewRouter(startedAt, services.tripDetails, orionmaxInventoryChangeService, services.read,
		services.cache, rabbitMQManagementReader, queueJobsService, tripFreshnessService, tripHistoryService,
		tablesService, operatorRegistry, busmapAnalyticsService, logsService, uiAccessAuth, freshnessVerifier,
		searchLogger, busmapLogger, inventoryLogger)
	server := &http.Server{Addr: config.Address(), Handler: router}
	log.Printf("orbitplusmaster listening on %s", config.Address())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("orbitplusmaster server stopped unexpectedly: %v", err)
	}
}

// setupFileLogging makes the standard logger write to a dated file inside
// logDir in addition to stderr, so the admin Logs page has something to read.
// It returns a close function for the file. A failure here is non-fatal: the
// service keeps logging to stderr as before.
func setupFileLogging(logDir string) (func(), error) {
	if logDir == "" {
		return nil, fmt.Errorf("log directory is empty")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", logDir, err)
	}
	name := filepath.Join(logDir, "orbitplusmaster-"+time.Now().UTC().Format("2006-01-02")+".log")
	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", name, err)
	}
	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return func() { _ = file.Close() }, nil
}

// newNamedFileLogger creates a dedicated dated log file inside logDir and
// returns a *log.Logger that writes to both stderr and that file, so a specific
// concern (e.g. search or busmap calls) has its own log file. A failure here is
// non-fatal: it falls back to the process logger and logs to stderr only.
func newNamedFileLogger(logDir, name string) (*log.Logger, func()) {
	if logDir == "" {
		return log.Default(), func() {}
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("%s file logging disabled: %v", name, err)
		return log.Default(), func() {}
	}
	fileName := filepath.Join(logDir, "orbitplusmaster-"+name+"-"+time.Now().UTC().Format("2006-01-02")+".log")
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("%s file logging disabled: %v", name, err)
		return log.Default(), func() {}
	}
	logger := log.New(io.MultiWriter(os.Stderr, file), "", log.LstdFlags)
	return logger, func() { _ = file.Close() }
}

// newCacheFreshnessVerifier builds the live Bits verification path when it is
// configured. A misconfigured or absent group disables only this feature: the
// service must keep serving cached reads, so nothing here is fatal.
func newCacheFreshnessVerifier(config master.RuntimeConfig, readService *master.TripDetailsReadService, repairer *master.TripDetailsStorage, differenceWriter master.CacheDifferenceWriter, rateLimiter master.ZoneRateLimiter) *master.CacheFreshnessVerifier {
	if config.VerificationError != nil {
		log.Printf("live verification disabled: %v", config.VerificationError)
		return nil
	}
	if config.Verification == nil {
		log.Print("live verification disabled: Cassandra/storage configuration is required")
		return nil
	}
	bitsClient, err := bits.NewBitsTripDetailsClient(
		&http.Client{Timeout: config.Verification.HTTPTimeout},
		config.AppEnvironment,
	)
	if err != nil {
		log.Printf("live verification disabled: %v", err)
		return nil
	}

	verifier, err := master.NewCacheFreshnessVerifier(bitsClient, readService, differenceWriter,
		repairer, rateLimiter, config.Verification.MaxConcurrent, log.Default())
	if err != nil {
		log.Printf("live verification disabled: %v", err)
		return nil
	}
	log.Printf("live verification enabled: max_concurrent=%d http_timeout=%s rate_limit=%d/%s recording=%t repair=%t credentials=request",
		config.Verification.MaxConcurrent, config.Verification.HTTPTimeout,
		config.Verification.RateLimit.Default.Hits, config.Verification.RateLimit.Default.Window,
		differenceWriter != nil, repairer != nil)
	return verifier
}

func newMasterServices(config master.RuntimeConfig) (masterServices, error) {
	if config.Storage == nil {
		log.Print("TripDetails persistence and queue metrix tracking are disabled: ingestion is log-only and persisted reads are unavailable")
		return masterServices{tripDetails: master.NewTripDetailsService(), close: func() {}}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Storage.Cassandra.Timeout)
	defer cancel()
	cache, err := dragonfly.NewTripDetailsCacheRepository(ctx, dragonfly.Config{
		Address: config.Storage.Dragonfly.Address, Password: config.Storage.Dragonfly.Password,
		Database: config.Storage.Dragonfly.Database, DialTimeout: config.Storage.Dragonfly.DialTimeout,
	})
	if err != nil {
		return masterServices{}, err
	}
	var rateLimiter master.ZoneRateLimiter
	if config.Verification != nil && config.Verification.RateLimit.Enabled() {
		limiter, limiterErr := dragonfly.NewZoneRateLimiter(cache, config.Verification.RateLimit)
		if limiterErr != nil {
			_ = cache.Close()
			return masterServices{}, limiterErr
		}
		rateLimiter = limiter
	}
	cassandraConfig := cassandra.Config{
		Hosts: config.Storage.Cassandra.Hosts, Port: config.Storage.Cassandra.Port,
		Keyspace: config.Storage.Cassandra.Keyspace, Username: config.Storage.Cassandra.Username,
		Password: config.Storage.Cassandra.Password, Timeout: config.Storage.Cassandra.Timeout,
	}
	metadata, err := cassandra.NewTripDetailsMetadataRepository(ctx, cassandraConfig)
	if err != nil {
		_ = cache.Close()
		return masterServices{}, err
	}
	metrix, err := cassandra.NewQueueMetrixRepository(ctx, cassandraConfig)
	if err != nil {
		metadata.Close()
		_ = cache.Close()
		return masterServices{}, err
	}
	persistence := master.NewTripDetailsStorageWithLogger(cache, metadata, log.Default())

	// The difference store is optional: a failure here disables live-difference
	// recording and the analytics report, but must not stop cached reads.
	var differences *cassandra.CacheFreshnessDifferenceRepository
	if repository, repositoryErr := cassandra.NewCacheFreshnessDifferenceRepository(ctx, cassandraConfig); repositoryErr != nil {
		log.Printf("busmap data analytics disabled: %v", repositoryErr)
	} else {
		differences = repository
	}

	return masterServices{
		tripDetails: master.NewTripDetailsServiceWithStorageAndMetrix(log.Default(), persistence, metrix),
		read:        master.NewTripDetailsReadService(cache, metadata, log.Default()),
		cache:       master.NewCacheReadService(cache),
		persistence: persistence,
		metadata:    metadata,
		metrix:      metrix,
		differences: differences,
		rateLimiter: rateLimiter,
		close: func() {
			if differences != nil {
				differences.Close()
			}
			metrix.Close()
			metadata.Close()
			_ = cache.Close()
		},
	}, nil
}
