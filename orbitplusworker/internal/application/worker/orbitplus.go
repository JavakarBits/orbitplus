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

// TripDetailsDLQRequest identifies a failed refresh for OrbitPlus-managed DLQ handling.
type TripDetailsDLQRequest struct {
	ReferenceID    string
	FailureMessage string
}

type OrbitPlusStatus string

const (
	OrbitPlusAccepted  OrbitPlusStatus = "ACCEPTED"
	OrbitPlusRetryable OrbitPlusStatus = "RETRYABLE"

	// OrbitPlusDuplicate and OrbitPlusStale are future-phase outcomes. They are
	// not currently producible by the existing OrbitPlus destination and are
	// defined here only for forward reference. Do not add them to
	// AcknowledgementEligible until the destination contract supports them.
	OrbitPlusDuplicate OrbitPlusStatus = "DUPLICATE"
	OrbitPlusStale     OrbitPlusStatus = "STALE"
)

// AcknowledgementEligible reports the terminal OrbitPlus outcomes that may ACK
// a delivery. In the current phase, only ACCEPTED is producible by the
// existing OrbitPlus destination.
func (status OrbitPlusStatus) AcknowledgementEligible() bool {
	return status == OrbitPlusAccepted
}

// OrbitPlusClient submits successful TripDetails and terminal failures to OrbitPlus.
type OrbitPlusClient interface {
	SendTripDetails(ctx context.Context, request TripDetailsRefreshRequest) (OrbitPlusStatus, error)
	SendTripDetailsDLQ(ctx context.Context, request TripDetailsDLQRequest) error
}
