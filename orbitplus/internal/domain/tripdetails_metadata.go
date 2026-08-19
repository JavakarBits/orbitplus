package domain

import "time"

// TripDetailsStageMetadata indexes one persisted TripDetails stage without
// retaining its JSON content in Cassandra.
type TripDetailsStageMetadata struct {
	OperatorCode    string
	TripCode        string
	ScheduleCode    string
	TravelDate      string
	FromStationCode string
	ToStationCode   string
	TripStageCode   string
	UpdatedAt       time.Time
}
