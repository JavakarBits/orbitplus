// orbitplusmaster runs the HTTP service that receives and serves TripDetails.
package main

import (
	"context"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/cassandra"
	"orbitplusmaster/internal/infrastructure/dragonfly"
	masterhttp "orbitplusmaster/internal/infrastructure/http"
	"orbitplusmaster/internal/infrastructure/rabbitmq"
)

func main() {
	log.Print("beginning orbitplusmaster startup")
	config, err := master.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("invalid orbitplusmaster configuration: %v", err)
	}
	log.Printf("orbitplusmaster configuration loaded: APP_ENV=%s MASTER_API_PORT=%d", config.AppEnvironment, config.APIPort)

	tripDetailsService, readService, metadata, metrix, closePersistence, err := newMasterServices(config)
	if err != nil {
		log.Fatalf("initialize TripDetails persistence: %v", err)
	}
	defer closePersistence()

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

	orionmaxInventoryChangeService := master.NewOrionmaxInventoryEventService(inventoryPublisher, metadata, metrix)
	queueJobsService := master.NewQueueJobsService(metrix)
	tablesService := master.NewTablesService(metadata)
	uiAccessAuth := masterhttp.NewUIAccessAuth(config.UIAccessToken, config.AppEnvironment == master.Production)
	router := masterhttp.NewRouter(tripDetailsService, orionmaxInventoryChangeService, readService, queueJobsService, tablesService, uiAccessAuth)
	server := &http.Server{Addr: config.Address(), Handler: router}
	log.Printf("orbitplusmaster listening on %s", config.Address())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("orbitplusmaster server stopped unexpectedly: %v", err)
	}
}

func newMasterServices(config master.RuntimeConfig) (*master.TripDetailsService, *master.TripDetailsReadService, *cassandra.TripDetailsMetadataRepository, *cassandra.QueueMetrixRepository, func(), error) {
	if config.Storage == nil {
		log.Print("TripDetails persistence and queue metrix tracking are disabled: ingestion is log-only and persisted reads are unavailable")
		return master.NewTripDetailsService(), nil, nil, nil, func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Storage.Cassandra.Timeout)
	defer cancel()
	cache, err := dragonfly.NewTripDetailsCacheRepository(ctx, dragonfly.Config{
		Address: config.Storage.Dragonfly.Address, Password: config.Storage.Dragonfly.Password,
		Database: config.Storage.Dragonfly.Database, DialTimeout: config.Storage.Dragonfly.DialTimeout,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cassandraConfig := cassandra.Config{
		Hosts: config.Storage.Cassandra.Hosts, Port: config.Storage.Cassandra.Port,
		Keyspace: config.Storage.Cassandra.Keyspace, Username: config.Storage.Cassandra.Username,
		Password: config.Storage.Cassandra.Password, Timeout: config.Storage.Cassandra.Timeout,
	}
	metadata, err := cassandra.NewTripDetailsMetadataRepository(ctx, cassandraConfig)
	if err != nil {
		_ = cache.Close()
		return nil, nil, nil, nil, nil, err
	}
	metrix, err := cassandra.NewQueueMetrixRepository(ctx, cassandraConfig)
	if err != nil {
		metadata.Close()
		_ = cache.Close()
		return nil, nil, nil, nil, nil, err
	}
	persistence := master.NewTripDetailsStorageWithLogger(cache, metadata, log.Default())
	readService := master.NewTripDetailsReadService(cache, metadata, log.Default())
	closePersistence := func() {
		metrix.Close()
		metadata.Close()
		_ = cache.Close()
	}
	return master.NewTripDetailsServiceWithStorageAndMetrix(log.Default(), persistence, metrix), readService, metadata, metrix, closePersistence, nil
}
