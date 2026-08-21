package master

import (
	"context"
	"errors"
	"time"

	"orbitplusmaster/internal/domain"
)

const tripFreshnessDateLayout = "2006-01-02"

// FutureTripMetadataReader reads route metadata used for the read-only dashboard freshness summary.
type FutureTripMetadataReader interface {
	ListRouteMetadata(context.Context) ([]domain.TripDetailsStageMetadata, error)
}

// TripFreshnessService derives future-trip stage freshness from persisted route metadata.
type TripFreshnessService struct {
	metadata FutureTripMetadataReader
	now      func() time.Time
}

// TripFreshnessSummary contains future logical trip-stage counts split by persistence freshness.
type TripFreshnessSummary struct {
	ActiveTripStages int       `json:"activeTripStages"`
	Fresh            int       `json:"fresh"`
	Aging            int       `json:"aging"`
	Stale            int       `json:"stale"`
	Critical         int       `json:"critical"`
	ObservedAt       time.Time `json:"observedAt"`
	Scope            string    `json:"scope"`
}

// NewTripFreshnessService constructs the read-only future-trip freshness service.
func NewTripFreshnessService(metadata FutureTripMetadataReader) *TripFreshnessService {
	return &TripFreshnessService{metadata: metadata, now: time.Now}
}

// Summary returns unique route stages whose ISO trip date is after the current UTC date.
func (service *TripFreshnessService) Summary(ctx context.Context) (TripFreshnessSummary, error) {
	if service == nil || service.metadata == nil {
		return TripFreshnessSummary{}, errors.New("trip freshness reporting is not configured")
	}
	now := service.now().UTC()
	rows, err := service.metadata.ListRouteMetadata(ctx)
	if err != nil {
		return TripFreshnessSummary{}, err
	}
	latest := make(map[tripFreshnessKey]time.Time)
	for _, row := range rows {
		if !isFutureTripDate(row.TripDate, now) {
			continue
		}
		key := tripFreshnessKey{operatorCode: row.OperatorCode, tripDate: row.TripDate, tripCode: row.TripCode, tripStageCode: row.TripStageCode}
		if updatedAt, exists := latest[key]; !exists || row.UpdatedAt.After(updatedAt) {
			latest[key] = row.UpdatedAt
		}
	}
	summary := TripFreshnessSummary{ActiveTripStages: len(latest), ObservedAt: now, Scope: "Unique route trip stages with trip dates after today; freshness uses the latest persisted route metadata update."}
	for _, updatedAt := range latest {
		age := now.Sub(updatedAt)
		switch {
		case age <= 10*time.Minute:
			summary.Fresh++
		case age <= 30*time.Minute:
			summary.Aging++
		case age <= time.Hour:
			summary.Stale++
		default:
			summary.Critical++
		}
	}
	return summary, nil
}

type tripFreshnessKey struct {
	operatorCode  string
	tripDate      string
	tripCode      string
	tripStageCode string
}

func isFutureTripDate(value string, now time.Time) bool {
	tripDate, err := time.Parse(tripFreshnessDateLayout, value)
	if err != nil {
		return false
	}
	return tripDate.After(now.UTC().Truncate(24 * time.Hour))
}
