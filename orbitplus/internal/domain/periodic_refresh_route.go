package domain

import "time"

// PeriodicRefreshRoute identifies one active route that should be refreshed.
type PeriodicRefreshRoute struct {
	OperatorCode string
	TravelDate   string
	FromStation  string
	ToStation    string
	TicketCount  int
	IsActive     bool
	UpdatedAt    time.Time
}
