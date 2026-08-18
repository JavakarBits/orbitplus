package master

import (
	"context"
	"log"
)

// TripDetailsService logs accepted payloads and persists cacheable TripDetails
// when the Phase 2 persistence dependency is configured.
type TripDetailsService struct {
	logger      *log.Logger
	persistence *TripDetailsPersistence
}

// NewTripDetailsService constructs a log-only TripDetailsService.
func NewTripDetailsService() *TripDetailsService {
	return NewTripDetailsServiceWithLogger(log.Default())
}

// NewTripDetailsServiceWithLogger constructs a log-only service with logger.
func NewTripDetailsServiceWithLogger(logger *log.Logger) *TripDetailsService {
	return NewTripDetailsServiceWithPersistence(logger, nil)
}

// NewTripDetailsServiceWithPersistence constructs a service with persistence.
func NewTripDetailsServiceWithPersistence(logger *log.Logger, persistence *TripDetailsPersistence) *TripDetailsService {
	return &TripDetailsService{logger: logger, persistence: persistence}
}

// ReceiveTripDetails logs original bytes and persists the validated JSON value.
func (service *TripDetailsService) ReceiveTripDetails(rawBody []byte, value any) error {
	service.logger.Print("TripDetails request received")
	service.logger.Printf("TripDetails payload: %s", rawBody)
	if service.persistence != nil {
		if err := service.persistence.Persist(context.Background(), value); err != nil {
			return err
		}
	}
	service.logger.Print("TripDetails request completed successfully")
	return nil
}
