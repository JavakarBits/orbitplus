package master

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"orbitplusmaster/internal/domain"
)

const periodicRefreshActivityType = "periodic-refresh"

// PeriodicRefreshRouteReader lists and saves routes used by periodic refresh.
type PeriodicRefreshRouteReader interface {
	ListActivePeriodicRefreshRoutes(context.Context) ([]domain.PeriodicRefreshRoute, error)
	ListPeriodicRefreshRoutes(context.Context) ([]domain.PeriodicRefreshRoute, error)
	SavePeriodicRefreshRoute(context.Context, domain.PeriodicRefreshRoute) error
}

// PeriodicRefreshMetadataReader discovers routes whose TripDetails have become stale.
type PeriodicRefreshMetadataReader interface {
	ListStaleRouteMetadata(context.Context, time.Time) ([]domain.TripDetailsStageMetadata, error)
}

// PeriodicRefreshPublisher publishes a periodic Worker refresh job.
type PeriodicRefreshPublisher interface {
	PublishPeriodicRefreshEvent(context.Context, string, []byte, int) error
}

// PeriodicRefreshScheduler serially queues active routes on a configured interval.
type PeriodicRefreshScheduler struct {
	logger    *log.Logger
	routes    PeriodicRefreshRouteReader
	metadata  PeriodicRefreshMetadataReader
	publisher PeriodicRefreshPublisher
	metrix    QueueMetrixStorage
	interval  time.Duration
}

// NewPeriodicRefreshScheduler constructs a scheduler for one service process.
func NewPeriodicRefreshScheduler(interval time.Duration, routes PeriodicRefreshRouteReader, metadata PeriodicRefreshMetadataReader, publisher PeriodicRefreshPublisher, metrix QueueMetrixStorage) *PeriodicRefreshScheduler {
	return &PeriodicRefreshScheduler{
		logger: log.Default(), routes: routes, metadata: metadata, publisher: publisher, metrix: metrix, interval: interval,
	}
}

// Start begins a serial ticker and returns a function that stops it.
func (scheduler *PeriodicRefreshScheduler) Start(parent context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	scheduler.logger.Printf("periodic refresh scheduler started: interval=%s first_run_after=%s", scheduler.interval, scheduler.interval)
	go scheduler.run(ctx)
	return cancel
}

func (scheduler *PeriodicRefreshScheduler) run(ctx context.Context) {
	ticker := time.NewTicker(scheduler.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			scheduler.logger.Print("periodic refresh scheduler stopped")
			return
		case tickAt := <-ticker.C:
			scheduler.logger.Printf("periodic refresh scheduler tick received: tick_at=%s", tickAt.UTC().Format(time.RFC3339))
			scheduler.Refresh(ctx)
		}
	}
}

// Refresh queues configured routes, then discovers stale unconfigured routes.
func (scheduler *PeriodicRefreshScheduler) Refresh(ctx context.Context) {
	startedAt := time.Now().UTC()
	scheduler.logger.Printf("periodic refresh cycle started: interval=%s started_at=%s", scheduler.interval, startedAt.Format(time.RFC3339))

	routes, err := scheduler.routes.ListActivePeriodicRefreshRoutes(ctx)
	if err != nil {
		scheduler.logger.Printf("periodic refresh cycle stopped: stage=read_active_routes error=%v", err)
		return
	}
	scheduler.logger.Printf("periodic refresh configured routes loaded: count=%d", len(routes))
	configuredQueued := 0
	for _, route := range routes {
		if scheduler.queueRoute(ctx, route, "configured") {
			configuredQueued++
		}
	}

	stale := scheduler.queueStaleUnconfiguredRoutes(ctx)
	scheduler.logger.Printf("periodic refresh cycle completed: configured_routes=%d configured_queued=%d stale_candidates=%d stale_skipped_existing=%d stale_skipped_invalid=%d stale_queue_failed=%d stale_routes_added=%d duration=%s", len(routes), configuredQueued, stale.candidates, stale.skippedExisting, stale.skippedInvalid, stale.queueFailed, stale.routesAdded, time.Since(startedAt))
}

type periodicRefreshStaleSummary struct {
	candidates      int
	skippedExisting int
	skippedInvalid  int
	queueFailed     int
	routesAdded     int
}

func (scheduler *PeriodicRefreshScheduler) queueStaleUnconfiguredRoutes(ctx context.Context) periodicRefreshStaleSummary {
	summary := periodicRefreshStaleSummary{}
	if scheduler.metadata == nil {
		scheduler.logger.Print("periodic refresh stale-route discovery skipped: metadata reader is not configured")
		return summary
	}
	configuredRoutes, err := scheduler.routes.ListPeriodicRefreshRoutes(ctx)
	if err != nil {
		scheduler.logger.Printf("periodic refresh stale-route discovery stopped: stage=read_configured_routes error=%v", err)
		return summary
	}
	cutoff := time.Now().UTC().Add(-time.Hour)
	scheduler.logger.Printf("periodic refresh stale-route discovery started: configured_routes=%d stale_cutoff=%s", len(configuredRoutes), cutoff.Format(time.RFC3339))
	staleMetadata, err := scheduler.metadata.ListStaleRouteMetadata(ctx, cutoff)
	if err != nil {
		scheduler.logger.Printf("periodic refresh stale-route discovery stopped: stage=read_stale_metadata cutoff=%s error=%v", cutoff.Format(time.RFC3339), err)
		return summary
	}
	summary.candidates = len(staleMetadata)
	scheduler.logger.Printf("periodic refresh stale metadata loaded: candidate_routes=%d", summary.candidates)

	existing := make(map[periodicRefreshRouteKey]struct{}, len(configuredRoutes))
	for _, route := range configuredRoutes {
		existing[newPeriodicRefreshRouteKey(route.OperatorCode, route.TravelDate, route.FromStation, route.ToStation)] = struct{}{}
	}
	for _, metadata := range staleMetadata {
		key := newPeriodicRefreshRouteKey(metadata.OperatorCode, metadata.TripDate, metadata.FromStationCode, metadata.ToStationCode)
		if !key.valid() {
			summary.skippedInvalid++
			scheduler.logger.Printf("periodic refresh stale route skipped: reason=incomplete_route metadata=%+v", metadata)
			continue
		}
		if _, exists := existing[key]; exists {
			summary.skippedExisting++
			scheduler.logger.Printf("periodic refresh stale route skipped: reason=already_configured operator=%q travel_date=%q from=%q to=%q", key.operatorCode, key.travelDate, key.fromStation, key.toStation)
			continue
		}
		route := domain.PeriodicRefreshRoute{
			OperatorCode: key.operatorCode, TravelDate: key.travelDate, FromStation: key.fromStation,
			ToStation: key.toStation, TicketCount: 0, IsActive: true, UpdatedAt: time.Now().UTC(),
		}
		if !scheduler.queueRoute(ctx, route, "stale_discovered") {
			summary.queueFailed++
			continue
		}
		if err := scheduler.routes.SavePeriodicRefreshRoute(ctx, route); err != nil {
			summary.queueFailed++
			scheduler.logger.Printf("periodic refresh discovered route save failed: operator=%q travel_date=%q from=%q to=%q error=%v", route.OperatorCode, route.TravelDate, route.FromStation, route.ToStation, err)
			continue
		}
		summary.routesAdded++
		existing[key] = struct{}{}
		scheduler.logger.Printf("periodic refresh discovered route added: operator=%q travel_date=%q from=%q to=%q ticket_count=0 is_active=true", route.OperatorCode, route.TravelDate, route.FromStation, route.ToStation)
	}
	return summary
}

type periodicRefreshRouteKey struct {
	operatorCode string
	travelDate   string
	fromStation  string
	toStation    string
}

func newPeriodicRefreshRouteKey(operatorCode, travelDate, fromStation, toStation string) periodicRefreshRouteKey {
	return periodicRefreshRouteKey{operatorCode: operatorCode, travelDate: travelDate, fromStation: fromStation, toStation: toStation}
}

func (key periodicRefreshRouteKey) valid() bool {
	return key.operatorCode != "" && key.travelDate != "" && key.fromStation != "" && key.toStation != ""
}

func (scheduler *PeriodicRefreshScheduler) queueRoute(ctx context.Context, route domain.PeriodicRefreshRoute, source string) bool {
	referenceID, err := newPeriodicRefreshReferenceID()
	if err != nil {
		scheduler.logger.Printf("periodic refresh reference ID creation failed: source=%s error=%v", source, err)
		return false
	}
	scheduler.logger.Printf("periodic refresh route queue started: source=%s reference_id=%q operator=%q travel_date=%q from=%q to=%q ticket_count=%d", source, referenceID, route.OperatorCode, route.TravelDate, route.FromStation, route.ToStation, route.TicketCount)
	now := time.Now().UTC()
	metric := domain.QueueMetrix{
		QueueID: referenceID, ActivityType: periodicRefreshActivityType, ActionType: "searchbusmap",
		OperatorCode: route.OperatorCode, SourceStationCode: route.FromStation,
		DestinationStationCode: route.ToStation, TripDate: route.TravelDate,
		QueueStatus: domain.QueueStatusReceived, UpdatedAt: now,
	}
	if err := scheduler.metrix.SaveReceived(ctx, metric); err != nil {
		scheduler.logger.Printf("periodic refresh route queue failed: source=%s reference_id=%q stage=save_received error=%v", source, referenceID, err)
		return false
	}
	scheduler.logger.Printf("periodic refresh route lifecycle saved: source=%s reference_id=%q status=RECEIVED", source, referenceID)
	payload, err := json.Marshal(struct {
		ReferenceID  string `json:"referenceId"`
		OperatorCode string `json:"operatorCode"`
		ActionType   string `json:"actionType"`
		FromCode     string `json:"fromCode"`
		ToCode       string `json:"toCode"`
		TripDate     string `json:"tripDate"`
	}{
		ReferenceID: referenceID, OperatorCode: route.OperatorCode, ActionType: "searchbusmap",
		FromCode: route.FromStation, ToCode: route.ToStation, TripDate: route.TravelDate,
	})
	if err != nil {
		scheduler.markDead(ctx, metric, err)
		scheduler.logger.Printf("periodic refresh route queue failed: source=%s reference_id=%q stage=build_payload error=%v", source, referenceID, err)
		return false
	}
	if err := scheduler.publisher.PublishPeriodicRefreshEvent(ctx, referenceID, payload, route.TicketCount); err != nil {
		scheduler.markDead(ctx, metric, err)
		scheduler.logger.Printf("periodic refresh route queue failed: source=%s reference_id=%q stage=publish error=%v", source, referenceID, err)
		return false
	}
	scheduler.logger.Printf("periodic refresh route published: source=%s reference_id=%q", source, referenceID)
	now = time.Now().UTC()
	metric.QueueStatus = domain.QueueStatusQueued
	metric.QueuedAt = now
	metric.UpdatedAt = now
	if err := scheduler.metrix.MarkQueued(ctx, metric); err != nil {
		scheduler.logger.Printf("periodic refresh route published with lifecycle update failure: source=%s reference_id=%q stage=mark_queued error=%v", source, referenceID, err)
		return true
	}
	scheduler.logger.Printf("periodic refresh route queued: source=%s reference_id=%q status=QUEUED", source, referenceID)
	return true
}

func (scheduler *PeriodicRefreshScheduler) markDead(ctx context.Context, metric domain.QueueMetrix, cause error) {
	now := time.Now().UTC()
	metric.QueueStatus = domain.QueueStatusDead
	metric.DeadLetteredAt = now
	metric.FailureMessage = queueMetrixFailureReason(cause)
	metric.UpdatedAt = now
	if err := scheduler.metrix.MarkDead(ctx, metric); err != nil {
		scheduler.logger.Printf("periodic refresh dead-state update failed: queue_id=%q error=%v", metric.QueueID, err)
	}
}

func newPeriodicRefreshReferenceID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate periodic refresh reference ID: %w", err)
	}
	return "periodic-refresh-" + hex.EncodeToString(value[:]), nil
}
