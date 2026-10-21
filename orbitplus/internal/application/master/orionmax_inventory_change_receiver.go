package master

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"orbitplusmaster/internal/domain"
)

// defaultInventoryRoutingKeyPrefix is the fallback routing-key prefix when
// RABBITMQ_ROUTING_KEY_PREFIX is unset. Each zone group is routed to its own
// queue by appending ".<group>" to the prefix (see RoutingKeyForZone).
const defaultInventoryRoutingKeyPrefix = "tripdetails.refresh"

// defaultZoneGroupName is the fallback group for any zone not listed as a
// dedicated group. Used when RABBITMQ_DEFAULT_ZONE_GROUP is unset.
const defaultZoneGroupName = "default"

// InventoryRefreshRoutingKey is the base routing key prefix, read from
// RABBITMQ_ROUTING_KEY_PREFIX (falling back to the default). It is a function
// rather than a constant so the prefix is configurable per deployment.
func InventoryRefreshRoutingKey() string {
	if prefix := strings.TrimSpace(os.Getenv("RABBITMQ_ROUTING_KEY_PREFIX")); prefix != "" {
		return strings.TrimRight(prefix, ".")
	}
	return defaultInventoryRoutingKeyPrefix
}

// dedicatedZoneGroups returns the set of zones that each have their own queue and
// worker, read from RABBITMQ_DEDICATED_ZONES (comma-separated zone codes). A zone
// not in this set is routed to the default group. An empty/unset value means no
// dedicated zones, so every zone routes to the default queue.
func dedicatedZoneGroups() map[string]struct{} {
	groups := make(map[string]struct{})
	for _, zone := range strings.Split(os.Getenv("RABBITMQ_DEDICATED_ZONES"), ",") {
		if normalized := strings.ToLower(strings.TrimSpace(zone)); normalized != "" {
			groups[normalized] = struct{}{}
		}
	}
	return groups
}

// defaultZoneGroup returns the catch-all group name, read from
// RABBITMQ_DEFAULT_ZONE_GROUP (falling back to the default).
func defaultZoneGroup() string {
	if group := strings.TrimSpace(os.Getenv("RABBITMQ_DEFAULT_ZONE_GROUP")); group != "" {
		return strings.ToLower(group)
	}
	return defaultZoneGroupName
}

// RoutingKeyForZone maps a zone code to the routing key of the queue that serves
// it. A zone listed in RABBITMQ_DEDICATED_ZONES gets its own queue; every other
// zone shares the default queue. The returned key is "<prefix>.<group>".
func RoutingKeyForZone(zoneCode string) string {
	group := defaultZoneGroup()
	normalized := strings.ToLower(strings.TrimSpace(zoneCode))
	if _, dedicated := dedicatedZoneGroups()[normalized]; dedicated {
		group = normalized
	}
	return InventoryRefreshRoutingKey() + "." + group
}

// InventoryEventPublisher publishes one prepared Worker job. The priority
// variant lets the receiver rank jobs by activity type; higher is served first.
// The routing-key variant selects the per-zone queue the job is routed to.
type InventoryEventPublisher interface {
	PublishInventoryEvent(context.Context, string, []byte) error
	PublishInventoryEventWithPriority(context.Context, string, []byte, uint8) error
	PublishInventoryEventWithPriorityAndRoutingKey(context.Context, string, []byte, uint8, string) error
}

// OrionmaxInventoryEventService receives and converts Orionmax inventory events.
type OrionmaxInventoryEventService struct {
	logger    *log.Logger
	publisher InventoryEventPublisher
	schedules InventoryScheduleReader
	metrix    QueueMetrixStorage
	operators OperatorRegistry
	// inFlight deduplicates concurrent/rapid duplicate inventory items: a route
	// already being queued is not queued again until it finishes. The key is the
	// route identity, not the per-event refid, so repeat events for the same
	// trip collapse into one in-flight job.
	inFlightMu sync.Mutex
	inFlight   map[string]struct{}
}

// NewOrionmaxInventoryEventService constructs an inventory-event receiver.
func NewOrionmaxInventoryEventService(publisher InventoryEventPublisher, schedules InventoryScheduleReader, metrix QueueMetrixStorage, operators OperatorRegistry) *OrionmaxInventoryEventService {
	return &OrionmaxInventoryEventService{logger: log.Default(), publisher: publisher, schedules: schedules, metrix: metrix, operators: operators, inFlight: make(map[string]struct{})}
}

// WithLogger directs inventory-change logging to a dedicated logger (e.g. its
// own log file) instead of the process default. A nil logger is ignored so the
// default remains in place.
func (service *OrionmaxInventoryEventService) WithLogger(logger *log.Logger) *OrionmaxInventoryEventService {
	if logger != nil {
		service.logger = logger
	}
	return service
}

// dedupKey identifies one route+trip so duplicate inventory items collapse.
func dedupKey(zone string, item orionmaxInventoryItem) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(zone)),
		strings.TrimSpace(item.OperatorCode),
		strings.TrimSpace(item.Source),
		strings.TrimSpace(item.Destination),
		strings.TrimSpace(item.DOJ),
		strings.TrimSpace(item.TripCode),
	}, "|")
}

// claimInFlight marks a route as being queued. It returns false when another
// job for the same route is already in flight, so the caller can skip it.
func (service *OrionmaxInventoryEventService) claimInFlight(key string) bool {
	service.inFlightMu.Lock()
	defer service.inFlightMu.Unlock()
	if _, exists := service.inFlight[key]; exists {
		return false
	}
	service.inFlight[key] = struct{}{}
	return true
}

// releaseInFlight clears the route once its job has reached a terminal state
// (queued, dead, or failed), so a later genuine change can be queued again.
func (service *OrionmaxInventoryEventService) releaseInFlight(key string) {
	service.inFlightMu.Lock()
	defer service.inFlightMu.Unlock()
	delete(service.inFlight, key)
}

// ReceiveInventoryChange logs, registers operators, and publishes active jobs.
func (service *OrionmaxInventoryEventService) ReceiveInventoryChange(ctx context.Context, activityType string, rawBody []byte) error {
	service.logger.Printf("Orionmax inventory change received: activity_type=%q bytes=%d payload=%s", activityType, len(rawBody), rawBody)
	if service.publisher == nil {
		return errors.New("inventory event publisher is not configured")
	}
	if service.metrix == nil {
		return errors.New("queue metrix storage is not configured")
	}
	actionType, event, err := decodeInventoryRefreshEvent(activityType, rawBody)
	if err != nil {
		return err
	}
	for _, item := range event.Data {
		if service.operators != nil {
			operator, err := service.operators.RegisterOperator(ctx, item.OperatorCode, event.Zone)
			if err != nil {
				return err
			}
			if !operator.Active() {
				service.logger.Printf("Orionmax inventory change skipped for inactive operator: operator_code=%q", operator.Code)
				continue
			}
		}
		now := time.Now().UTC()
		metric := newQueueMetrix(activityType, actionType, event.Zone, item, now)
		if metric.ReferenceID == "" {
			return ErrInvalidInventoryEvent
		}

		// Skip when the same route is already being queued, so a duplicate
		// inventory item does not produce a second worker job.
		key := dedupKey(event.Zone, item)
		if !service.claimInFlight(key) {
			service.logger.Printf("Orionmax inventory change skipped as duplicate: operator_code=%q zone=%q", item.OperatorCode, event.Zone)
			continue
		}

		job, err := buildInventoryRefreshJob(ctx, actionType, item, service.schedules, metric)
		if err != nil {
			if saveErr := service.metrix.SaveReceived(ctx, metric); saveErr != nil {
				service.releaseInFlight(key)
				return fmt.Errorf("save queue metrix record: %w", saveErr)
			}
			service.markDead(ctx, metric, err)
			service.releaseInFlight(key)
			return err
		}
		if err := service.metrix.SaveReceived(ctx, job.Metric); err != nil {
			service.releaseInFlight(key)
			return fmt.Errorf("save queue metrix record: %w", err)
		}
		priority := priorityForActivityType(activityType)
		routingKey := RoutingKeyForZone(event.Zone)
		service.logger.Printf("RabbitMQ publish started: reference_id=%q operator_code=%q zone=%q routing_key=%q action=%q activity_type=%q priority=%d", job.Metric.ReferenceID, item.OperatorCode, event.Zone, routingKey, actionType, activityType, priority)
		if err := service.publisher.PublishInventoryEventWithPriorityAndRoutingKey(ctx, job.Metric.ReferenceID, job.Payload, priority, routingKey); err != nil {
			service.logger.Printf("RabbitMQ publish failed: reference_id=%q operator_code=%q error=%v", job.Metric.ReferenceID, item.OperatorCode, err)
			service.markDead(ctx, job.Metric, err)
			service.releaseInFlight(key)
			return err
		}
		service.logger.Printf("RabbitMQ publish succeeded: reference_id=%q operator_code=%q zone=%q routing_key=%q", job.Metric.ReferenceID, item.OperatorCode, event.Zone, routingKey)
		now = time.Now().UTC()
		job.Metric.QueueStatus = domain.QueueStatusQueued
		job.Metric.QueuedAt = now
		job.Metric.UpdatedAt = now
		if err := service.metrix.MarkQueued(ctx, job.Metric); err != nil {
			service.releaseInFlight(key)
			return fmt.Errorf("mark queue metrix queued: %w", err)
		}
		// The job is now safely on the broker. Release the route so a later
		// genuine change for it can be queued again.
		service.releaseInFlight(key)
	}
	return nil
}

func (service *OrionmaxInventoryEventService) markDead(ctx context.Context, metric domain.QueueMetrix, cause error) {
	now := time.Now().UTC()
	metric.QueueStatus = domain.QueueStatusDead
	metric.DeadLetteredAt = now
	metric.FailureMessage = queueMetrixFailureReason(cause)
	metric.UpdatedAt = now
	if err := service.metrix.MarkDead(ctx, metric); err != nil {
		service.logger.Printf("queue metrix dead-state update failed: reference_id=%q error=%v", metric.ReferenceID, err)
	}
}

func queueMetrixFailureReason(err error) string {
	const maxLength = 500
	message := err.Error()
	if len(message) > maxLength {
		return message[:maxLength]
	}
	return message
}
