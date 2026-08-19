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
)

type ExecutionResult struct {
	Status          ExecutionStatus
	OrbitPlusStatus OrbitPlusStatus
	Acknowledged    bool
	Err             error
}
