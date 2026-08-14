package worker

import (
	"context"

	"orbitplusworker/internal/domain"
)

// TripDetailsRefreshRequest is the raw Bits result and its action-specific
// message identity, sent only to the existing OrbitPlus destination contract.
type TripDetailsRefreshRequest struct {
	Message      domain.TripDetailsRefreshMessage
	BitsResponse []byte
}

type OrbitPlusStatus string

const (
	OrbitPlusAccepted  OrbitPlusStatus = "ACCEPTED"
	OrbitPlusDuplicate OrbitPlusStatus = "DUPLICATE"
	OrbitPlusStale     OrbitPlusStatus = "STALE"
	OrbitPlusRetryable OrbitPlusStatus = "RETRYABLE"
)

// AcknowledgementEligible reports the terminal OrbitPlus outcomes that may ACK a delivery.
func (status OrbitPlusStatus) AcknowledgementEligible() bool {
	return status == OrbitPlusAccepted || status == OrbitPlusDuplicate || status == OrbitPlusStale
}

// OrbitPlusClient submits TripDetails only to OrbitPlus.
type OrbitPlusClient interface {
	SendTripDetails(ctx context.Context, request TripDetailsRefreshRequest) (OrbitPlusStatus, error)
}
