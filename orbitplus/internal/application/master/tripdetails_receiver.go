package master

import (
	"context"
	"log"
)

// TripDetailsService logs accepted payloads and persists cacheable TripDetails
// when the Phase 2 persistence dependency is configured.
type TripDetailsService struct {
	logger      *log.Logger
	persistence *TripDetailsStorage
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
	return &TripDetailsService{logger: logger, persistence: persistence}
}

// ReceiveTripDetails logs original bytes and persists the validated JSON value.
func (service *TripDetailsService) ReceiveTripDetails(rawBody []byte, value any) error {
	service.logger.Print("TripDetails request received")
	service.logger.Printf("TripDetails payload: %s", rawBody)
	if service.persistence != nil {
		if err := service.persistence.Store(context.Background(), value); err != nil {
			return err
		}
	}
	service.logger.Print("TripDetails request completed successfully")
	return nil
}
