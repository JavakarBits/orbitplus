package master

import (
	"context"
	"errors"
	"log"
	"time"

	"orbitplusmaster/internal/domain"
)

// TripDetailsService logs accepted payloads and persists cacheable TripDetails
// when the Phase 2 persistence dependency is configured.
type TripDetailsService struct {
	logger      *log.Logger
	persistence *TripDetailsStorage
	metrix      QueueMetrixStorage
}

// NewTripDetailsService constructs a log-only TripDetailsService.
func NewTripDetailsService() *TripDetailsService {
	return NewTripDetailsServiceWithLogger(log.Default())
}

// NewTripDetailsServiceWithLogger constructs a log-only service with logger.
func NewTripDetailsServiceWithLogger(logger *log.Logger) *TripDetailsService {
	return NewTripDetailsServiceWithStorage(logger, nil)
}

// NewTripDetailsServiceWithStorage constructs a service with persistence.
func NewTripDetailsServiceWithStorage(logger *log.Logger, persistence *TripDetailsStorage) *TripDetailsService {
	return NewTripDetailsServiceWithStorageAndMetrix(logger, persistence, nil)
}

// NewTripDetailsServiceWithStorageAndMetrix constructs a service with storage
// and optional queue job lifecycle tracking.
func NewTripDetailsServiceWithStorageAndMetrix(logger *log.Logger, persistence *TripDetailsStorage, metrix QueueMetrixStorage) *TripDetailsService {
	return &TripDetailsService{logger: logger, persistence: persistence, metrix: metrix}
}

// ReceiveTripDetails logs original bytes, persists the validated JSON value,
// and completes the corresponding queue lifecycle record when supplied.
func (service *TripDetailsService) ReceiveTripDetails(rawBody []byte, value any) error {
	service.logger.Print("TripDetails request received")
	service.logger.Printf("TripDetails payload: %s", rawBody)
	referenceID := tripDetailsReferenceID(value)
	storeResult := tripDetailsStoreResult{}
	if service.persistence != nil {
		var err error
		storeResult, err = service.persistence.Store(context.Background(), value)
		if err != nil {
			service.markDead(referenceID, storeResult.UpdatedTripCodes, err)
			return err
		}
	}
	if referenceID != "" && service.metrix != nil {
		now := time.Now().UTC()
		if err := service.metrix.MarkCompleted(context.Background(), domain.QueueMetrix{
			ReferenceID: referenceID, QueueStatus: domain.QueueStatusCompleted,
			UpdatedTripCodes: storeResult.UpdatedTripCodes, CompletedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	service.logger.Print("TripDetails request completed successfully")
	return nil
}

// MarkTripDetailsDead records a Worker dead-letter notification for one queued job.
func (service *TripDetailsService) MarkTripDetailsDead(ctx context.Context, referenceID, failureMessage string) error {
	if service == nil || service.metrix == nil {
		return errors.New("queue metrix tracking is not configured")
	}
	now := time.Now().UTC()
	if err := service.metrix.MarkDead(ctx, domain.QueueMetrix{
		ReferenceID: referenceID, QueueStatus: domain.QueueStatusDead, DeadLetteredAt: now,
		FailureMessage: failureMessage, UpdatedAt: now,
	}); err != nil {
		return err
	}
	service.logger.Printf("TripDetails DLQ notification recorded: reference_id=%q", referenceID)
	return nil
}

func (service *TripDetailsService) markDead(referenceID string, updatedTripCodes []string, cause error) {
	if referenceID == "" || service.metrix == nil {
		return
	}
	now := time.Now().UTC()
	if err := service.metrix.MarkDead(context.Background(), domain.QueueMetrix{
		ReferenceID: referenceID, QueueStatus: domain.QueueStatusDead, DeadLetteredAt: now,
		UpdatedTripCodes: updatedTripCodes, FailureMessage: queueMetrixFailureReason(cause), UpdatedAt: now,
	}); err != nil {
		service.logger.Printf("queue metrix dead-state update failed: reference_id=%q error=%v", referenceID, err)
	}
}

func tripDetailsReferenceID(value any) string {
	root, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if referenceID := stringField(root, "referenceId"); referenceID != "" {
		return referenceID
	}
	return stringField(root, "refid")
}
