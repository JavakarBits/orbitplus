package master

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"orbitplusmaster/internal/domain"
)

const periodicRouteRefreshActivityType = "periodic-route-refresh"

// PriorityInventoryEventPublisher publishes one Worker job with an explicit AMQP priority.
type PriorityInventoryEventPublisher interface {
	PublishInventoryEventWithPriority(context.Context, string, []byte, uint8) error
}

// StaleRouteMetadataReader reads route metadata that has not been updated recently.
type StaleRouteMetadataReader interface {
	ListStaleRouteMetadata(context.Context, time.Time) ([]domain.TripDetailsStageMetadata, error)
}

type orbitTopRouteResponse struct {
	Data struct {
		Route map[string]orbitRouteDefinition `json:"route"`
	} `json:"data"`
}

type orbitRouteDefinition struct {
	Route    []string `json:"route"`
	TopRoute []string `json:"topRoute"`
}

// OrbitRouteRefreshService periodically refreshes stale future route pairs.
type OrbitRouteRefreshService struct {
	config    OrbitRouteRefreshConfig
	client    *http.Client
	logger    *log.Logger
	metadata  StaleRouteMetadataReader
	metrix    QueueMetrixStorage
	metrics   QueueMetrixReferenceReader
	operators OperatorRegistry
	publisher PriorityInventoryEventPublisher
	now       func() time.Time
	mutex     sync.Mutex
}

// NewOrbitRouteRefreshService constructs a stale-metadata periodic route refresh service.
func NewOrbitRouteRefreshService(config OrbitRouteRefreshConfig, metadata StaleRouteMetadataReader, metrix QueueMetrixStorage, metrics QueueMetrixReferenceReader, operators OperatorRegistry, publisher PriorityInventoryEventPublisher) *OrbitRouteRefreshService {
	return &OrbitRouteRefreshService{
		config: config, client: &http.Client{Timeout: config.Timeout}, logger: log.Default(), metadata: metadata,
		metrix: metrix, metrics: metrics, operators: operators, publisher: publisher, now: time.Now,
	}
}

// Start begins periodic execution without an immediate first run. Stop cancels its ticker.
func (service *OrbitRouteRefreshService) Start() func() {
	if service == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(service.config.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := service.Run(context.Background()); err != nil && !errors.Is(err, errRouteRefreshAlreadyRunning) {
					service.logger.Printf("Orbit route refresh failed: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}

var errRouteRefreshAlreadyRunning = errors.New("Orbit route refresh is already running")

// Run queues stale future routes that still exist in an active operator's Orbit route list.
func (service *OrbitRouteRefreshService) Run(ctx context.Context) error {
	if service == nil || service.metadata == nil || service.metrix == nil || service.metrics == nil || service.operators == nil || service.publisher == nil {
		return errors.New("Orbit route refresh is not configured")
	}
	if !service.mutex.TryLock() {
		return errRouteRefreshAlreadyRunning
	}
	defer service.mutex.Unlock()

	now := service.now().UTC()
	cutoff := now.Add(-service.config.StaleDuration)
	service.logger.Printf("Orbit periodic route refresh started: now=%s stale_cutoff=%s stale_duration=%s", now.Format(time.RFC3339), cutoff.Format(time.RFC3339), service.config.StaleDuration)

	activeOperators, err := service.operators.ListActiveOperators(ctx)
	if err != nil {
		service.logger.Printf("Orbit periodic route refresh failed: stage=list_active_operators error=%v", err)
		return fmt.Errorf("list active operators: %w", err)
	}
	active := make(map[string]struct{}, len(activeOperators))
	zoneURLs := make(map[string]string, len(activeOperators))
	for _, operator := range activeOperators {
		if !operator.Active() {
			continue
		}
		zoneURL, exists := zoneURLFor(operator.ZoneCode)
		if !exists {
			service.logger.Printf("Orbit periodic route refresh failed: stage=resolve_zone operator_code=%q zone_code=%q", operator.Code, operator.ZoneCode)
			return fmt.Errorf("Orbit route refresh zone is unavailable for active operator %q", operator.Code)
		}
		active[operator.Code] = struct{}{}
		zoneURLs[operator.Code] = zoneURL
		service.logger.Printf("Orbit periodic route refresh selected operator: operator_code=%q zone_code=%q", operator.Code, operator.ZoneCode)
	}
	service.logger.Printf("Orbit periodic route refresh active operators loaded: count=%d", len(active))

	metadata, err := service.metadata.ListStaleRouteMetadata(ctx, cutoff)
	if err != nil {
		service.logger.Printf("Orbit periodic route refresh failed: stage=list_stale_metadata error=%v", err)
		return fmt.Errorf("list stale route metadata: %w", err)
	}
	routesByOperator := service.futureRoutePairs(metadata, active, now)
	candidateCount := 0
	for _, routes := range routesByOperator {
		candidateCount += len(routes)
	}
	service.logger.Printf("Orbit periodic route refresh stale metadata loaded: rows=%d future_route_candidates=%d", len(metadata), candidateCount)

	queuedCount := 0
	skippedExistingCount := 0
	notInOrbitCount := 0
	topRouteCount := 0
	normalRouteCount := 0
	for operatorCode, routes := range routesByOperator {
		service.logger.Printf("Orbit periodic route refresh loading Orbit routes: operator_code=%q candidates=%d", operatorCode, len(routes))
		orbitRoutes, err := service.fetchOrbitRoutes(ctx, operatorCode, service.config.AccessToken)
		if err != nil {
			service.logger.Printf("Orbit periodic route refresh failed: stage=fetch_orbit_routes operator_code=%q error=%v", operatorCode, err)
			return fmt.Errorf("fetch Orbit routes for operator %q: %w", operatorCode, err)
		}
		service.logger.Printf("Orbit periodic route refresh Orbit routes loaded: operator_code=%q origin_station_count=%d", operatorCode, len(orbitRoutes))
		for _, route := range routes {
			priority, matches := orbitRoutePriority(orbitRoutes[route.fromStation], route.toStation)
			if !matches {
				notInOrbitCount++
				service.logger.Printf("Orbit periodic route refresh skipped route: reason=not_in_orbit_routes operator_code=%q trip_date=%q from_code=%q to_code=%q metadata_updated_at=%s", route.operatorCode, route.tripDate, route.fromStation, route.toStation, route.updatedAt.Format(time.RFC3339))
				continue
			}
			if priority == 9 {
				topRouteCount++
			} else {
				normalRouteCount++
			}
			queued, err := service.publishRoute(ctx, route, zoneURLs[operatorCode], priority)
			if err != nil {
				service.logger.Printf("Orbit periodic route refresh failed: stage=publish_route operator_code=%q trip_date=%q from_code=%q to_code=%q priority=%d error=%v", route.operatorCode, route.tripDate, route.fromStation, route.toStation, priority, err)
				return err
			}
			if !queued {
				skippedExistingCount++
				service.logger.Printf("Orbit periodic route refresh skipped route: reason=already_queued_or_completed operator_code=%q trip_date=%q from_code=%q to_code=%q priority=%d", route.operatorCode, route.tripDate, route.fromStation, route.toStation, priority)
				continue
			}
			queuedCount++
			service.logger.Printf("Orbit periodic route refresh queued route: operator_code=%q trip_date=%q from_code=%q to_code=%q priority=%d", route.operatorCode, route.tripDate, route.fromStation, route.toStation, priority)
		}
	}
	service.logger.Printf("Orbit periodic route refresh completed: active_operators=%d stale_metadata_rows=%d future_route_candidates=%d top_route_matches=%d normal_route_matches=%d not_in_orbit_routes=%d queued=%d skipped_existing=%d", len(active), len(metadata), candidateCount, topRouteCount, normalRouteCount, notInOrbitCount, queuedCount, skippedExistingCount)
	return nil
}

type routeRefreshPair struct {
	operatorCode string
	tripDate     string
	fromStation  string
	toStation    string
	updatedAt    time.Time
}

func (service *OrbitRouteRefreshService) futureRoutePairs(rows []domain.TripDetailsStageMetadata, active map[string]struct{}, now time.Time) map[string][]routeRefreshPair {
	today := now.UTC().Truncate(24 * time.Hour)
	latest := today.AddDate(0, 0, 7)
	pairs := make(map[string]routeRefreshPair)
	for _, row := range rows {
		if _, ok := active[row.OperatorCode]; !ok {
			continue
		}
		tripDate, err := time.Parse("2006-01-02", row.TripDate)
		if err != nil || tripDate.Before(today) || tripDate.After(latest) || row.FromStationCode == "" || row.ToStationCode == "" {
			continue
		}
		key := row.OperatorCode + "\x00" + row.TripDate + "\x00" + row.FromStationCode + "\x00" + row.ToStationCode
		candidate := routeRefreshPair{operatorCode: row.OperatorCode, tripDate: row.TripDate, fromStation: row.FromStationCode, toStation: row.ToStationCode, updatedAt: row.UpdatedAt.UTC()}
		if existing, found := pairs[key]; !found || candidate.updatedAt.After(existing.updatedAt) {
			pairs[key] = candidate
		}
	}
	result := make(map[string][]routeRefreshPair)
	for _, route := range pairs {
		result[route.operatorCode] = append(result[route.operatorCode], route)
	}
	return result
}

func (service *OrbitRouteRefreshService) fetchOrbitRoutes(ctx context.Context, operatorCode, accessToken string) (map[string]orbitRouteDefinition, error) {
	endpoint, err := orbitTopRouteURL(service.config.BaseURL, operatorCode, accessToken)
	if err != nil {
		return nil, errors.New("create Orbit route request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("create Orbit route request")
	}
	request.Header.Set("Accept", "application/json")
	response, err := service.client.Do(request)
	if err != nil {
		return nil, errors.New("request Orbit routes")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("Orbit routes returned HTTP %d", response.StatusCode)
	}
	var payload orbitTopRouteResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, errors.New("decode Orbit routes")
	}
	return payload.Data.Route, nil
}

func orbitTopRouteURL(baseURL, operatorCode, accessToken string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return "", errors.New("invalid Orbit route base URL")
	}
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + "/orbitservices/ezeeinfo/" + url.PathEscape(operatorCode) + "/" + url.PathEscape(accessToken) + "/top/route"
	return parsedURL.String(), nil
}

func orbitRoutePriority(route orbitRouteDefinition, toStation string) (uint8, bool) {
	if containsStation(route.TopRoute, toStation) {
		return 9, true
	}
	if containsStation(route.Route, toStation) {
		return 8, true
	}
	return 0, false
}

func (service *OrbitRouteRefreshService) publishRoute(ctx context.Context, route routeRefreshPair, zoneURL string, priority uint8) (bool, error) {
	referenceID := periodicRouteReferenceID(route)
	existing, found, err := service.metrics.FindByReferenceID(ctx, referenceID)
	if err != nil {
		return false, fmt.Errorf("find periodic queue metric: %w", err)
	}
	if found && (existing.QueueStatus == domain.QueueStatusQueued || existing.QueueStatus == domain.QueueStatusCompleted) {
		return false, nil
	}
	payload, err := json.Marshal(map[string]string{
		"referenceId": referenceID, "operatorCode": route.operatorCode, "actionType": "searchbusmap", "zoneURL": zoneURL,
		"fromCode": route.fromStation, "toCode": route.toStation, "tripDate": route.tripDate,
	})
	if err != nil {
		return false, fmt.Errorf("encode periodic Worker job: %w", err)
	}
	now := service.now().UTC()
	metric := domain.QueueMetrix{
		ReferenceID: referenceID, ActivityType: periodicRouteRefreshActivityType, ActionType: "searchbusmap", OperatorCode: route.operatorCode,
		SourceStationCode: route.fromStation, DestinationStationCode: route.toStation, TripDate: route.tripDate, Zone: zoneURL,
		WorkerPayload: payload, QueueStatus: domain.QueueStatusReceived, UpdatedAt: now,
	}
	if err := service.metrix.SaveReceived(ctx, metric); err != nil {
		return false, fmt.Errorf("save periodic queue metric: %w", err)
	}
	if err := service.publisher.PublishInventoryEventWithPriority(ctx, referenceID, payload, priority); err != nil {
		service.markDead(ctx, metric, err)
		return false, fmt.Errorf("publish periodic Worker job: %w", err)
	}
	metric.QueueStatus = domain.QueueStatusQueued
	metric.QueuedAt = service.now().UTC()
	metric.UpdatedAt = metric.QueuedAt
	if err := service.metrix.MarkQueued(ctx, metric); err != nil {
		return false, fmt.Errorf("mark periodic queue metric queued: %w", err)
	}
	return true, nil
}

func (service *OrbitRouteRefreshService) markDead(ctx context.Context, metric domain.QueueMetrix, cause error) {
	now := service.now().UTC()
	metric.QueueStatus = domain.QueueStatusDead
	metric.DeadLetteredAt = now
	metric.FailureMessage = queueMetrixFailureReason(cause)
	metric.UpdatedAt = now
	if err := service.metrix.MarkDead(ctx, metric); err != nil {
		service.logger.Printf("periodic queue metrix dead-state update failed: reference_id=%q error=%v", metric.ReferenceID, err)
	}
}

func periodicRouteReferenceID(route routeRefreshPair) string {
	updatedAt := route.updatedAt.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(strings.Join([]string{periodicRouteRefreshActivityType, route.operatorCode, route.tripDate, route.fromStation, route.toStation, updatedAt}, "\x00")))
	return "periodic-route-" + hex.EncodeToString(sum[:])
}

func containsStation(stations []string, station string) bool {
	for _, candidate := range stations {
		if candidate == station {
			return true
		}
	}
	return false
}
