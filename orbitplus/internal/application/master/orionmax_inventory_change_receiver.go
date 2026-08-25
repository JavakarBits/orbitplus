package master

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"orbitplusmaster/internal/domain"
)

const InventoryRefreshRoutingKey = "tripdetails.refresh"

// InventoryEventPublisher publishes one prepared Worker job.
type InventoryEventPublisher interface {
	PublishInventoryEvent(context.Context, string, []byte) error
}

// OrionmaxInventoryEventService receives and converts Orionmax inventory events.
type OrionmaxInventoryEventService struct {
	logger    *log.Logger
	publisher InventoryEventPublisher
	schedules InventoryScheduleReader
	metrix    QueueMetrixStorage
	operators OperatorRegistry
}

// NewOrionmaxInventoryEventService constructs an inventory-event receiver.
func NewOrionmaxInventoryEventService(publisher InventoryEventPublisher, schedules InventoryScheduleReader, metrix QueueMetrixStorage, operators OperatorRegistry) *OrionmaxInventoryEventService {
	return &OrionmaxInventoryEventService{logger: log.Default(), publisher: publisher, schedules: schedules, metrix: metrix, operators: operators}
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
		job, err := buildInventoryRefreshJob(ctx, actionType, item, service.schedules, metric)
		if err != nil {
			if saveErr := service.metrix.SaveReceived(ctx, metric); saveErr != nil {
				return fmt.Errorf("save queue metrix record: %w", saveErr)
			}
			service.markDead(ctx, metric, err)
			return err
		}
		if err := service.metrix.SaveReceived(ctx, job.Metric); err != nil {
			return fmt.Errorf("save queue metrix record: %w", err)
		}
		if err := service.publisher.PublishInventoryEvent(ctx, job.Metric.ReferenceID, job.Payload); err != nil {
			service.markDead(ctx, job.Metric, err)
			return err
		}
		now = time.Now().UTC()
		job.Metric.QueueStatus = domain.QueueStatusQueued
		job.Metric.QueuedAt = now
		job.Metric.UpdatedAt = now
		if err := service.metrix.MarkQueued(ctx, job.Metric); err != nil {
			return fmt.Errorf("mark queue metrix queued: %w", err)
		}
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
