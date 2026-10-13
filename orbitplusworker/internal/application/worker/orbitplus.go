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

// OrbitPlusHealthResult carries the details of an OrbitPlus health check so
// the caller can log clearly whether the server is down (unreachable) or
// reachable but unhealthy, and include the endpoint, status code, and body.
type OrbitPlusHealthResult struct {
	// Endpoint is the URL that was probed (e.g. https://orbitplus.example/health).
	Endpoint string
	// Reachable is true when an HTTP response was received (even if non-2xx).
	// False means the server is completely down or the network is unreachable.
	Reachable bool
	// Healthy is true only when the server responded with 2xx.
	Healthy bool
	// StatusCode is the HTTP status code, zero when the server is unreachable.
	StatusCode int
	// Body is the response body (trimmed), empty when unreachable.
	Body string
	// Detail carries the transport error message when Reachable is false.
	Detail string
}

// OrbitPlusClient submits successful TripDetails and terminal failures to OrbitPlus.
type OrbitPlusClient interface {
	// CheckHealth reports whether OrbitPlus is reachable and healthy. The worker
	// calls this before any outbound Bits request so a down OrbitPlus does not
	// cause an expensive Bits fetch whose result could never be delivered. A
	// non-nil error means OrbitPlus should be treated as unavailable; it is
	// retryable so the delivery is retried rather than dead-lettered.
	CheckHealth(ctx context.Context) (OrbitPlusHealthResult, error)
	SendTripDetails(ctx context.Context, request TripDetailsRefreshRequest) (OrbitPlusStatus, error)
	SendTripDetailsDLQ(ctx context.Context, request TripDetailsDLQRequest) error
}
