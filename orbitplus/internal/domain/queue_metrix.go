package domain

import (
	"encoding/json"
	"time"
)

const (
	QueueStatusReceived  = "RECEIVED"
	QueueStatusQueued    = "QUEUED"
	QueueStatusCompleted = "COMPLETED"
	QueueStatusDead      = "DEAD"
)

// QueueMetrix tracks one Orionmax inventory item through queue publication and
// the corresponding Worker result.
type QueueMetrix struct {
	ReferenceID            string
	ActivityType           string
	ActionType             string
	OperatorCode           string
	ScheduleCode           string
	TripCode               string
	SourceStationCode      string
	DestinationStationCode string
	TripDate               string
	Zone                   string
	WorkerPayload          json.RawMessage
	UpdatedTripCodes       []string
	QueueStatus            string
	QueuedAt               time.Time
	CompletedAt            time.Time
	DeadLetteredAt         time.Time
	FailureMessage         string
	UpdatedAt              time.Time
}
