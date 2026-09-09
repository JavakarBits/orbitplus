package worker

type ExecutionStatus string

const (
	ExecutionInvalidEnvelope  ExecutionStatus = "INVALID_ENVELOPE"
	ExecutionCredentialError  ExecutionStatus = "CREDENTIAL_ERROR"
	ExecutionSourceError      ExecutionStatus = "SOURCE_ERROR"
	ExecutionOrbitPlusError   ExecutionStatus = "ORBITPLUS_ERROR"
	ExecutionOrbitPlusOutcome ExecutionStatus = "ORBITPLUS_OUTCOME"
	ExecutionDLQReported      ExecutionStatus = "DLQ_REPORTED"
	ExecutionDLQError         ExecutionStatus = "DLQ_ERROR"
	ExecutionAcknowledged     ExecutionStatus = "ACKNOWLEDGED"
	ExecutionAckError         ExecutionStatus = "ACK_ERROR"
	ExecutionCancelled        ExecutionStatus = "CANCELLED"
	// ExecutionRateLimited means the zone was on cooldown, so the delivery was
	// requeued for later without calling BITS.
	ExecutionRateLimited ExecutionStatus = "RATE_LIMITED"
	// ExecutionRequeueError means the rate-limited delivery could not be
	// returned to the queue.
	ExecutionRequeueError ExecutionStatus = "REQUEUE_ERROR"
)

type ExecutionResult struct {
	Status          ExecutionStatus
	OrbitPlusStatus OrbitPlusStatus
	Acknowledged    bool
	Err             error
}
