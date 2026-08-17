package master

import "log"

// TripDetailsService contains the Phase 1 application logic for received
// TripDetails payloads: log what was received and acknowledge success.
// Future phases (persistence, splitting, freshness checks, etc.) will extend
// this service without changing the HTTP layer.
type TripDetailsService struct{}

// NewTripDetailsService constructs a TripDetailsService.
func NewTripDetailsService() *TripDetailsService {
	return &TripDetailsService{}
}

// ReceiveTripDetails logs the well-formed TripDetails payload received from
// the Worker. rawBody is the exact bytes received on the wire so that no
// information is lost when logging.
func (service *TripDetailsService) ReceiveTripDetails(rawBody []byte) {
	log.Print("TripDetails request received")
	log.Printf("TripDetails payload: %s", rawBody)
	log.Print("TripDetails request completed successfully")
}
