package master

import (
	"context"
	"errors"
	"strings"

	"orbitplusmaster/internal/domain"
)

var (
	// ErrTablesNotConfigured indicates that Cassandra-backed tables reporting is unavailable.
	ErrTablesNotConfigured = errors.New("tables reporting is not configured")
	// ErrInvalidTablesLookup indicates that a required tables lookup key is absent or unsafe.
	ErrInvalidTablesLookup = errors.New("invalid tables lookup")
)

// TablesPeriodicRefreshRouteReader reads all periodic refresh route partitions.
type TablesPeriodicRefreshRouteReader interface {
	ListPeriodicRefreshRoutes(context.Context) ([]domain.PeriodicRefreshRoute, error)
}

// TablesMetadataReader reads TripDetails metadata lookup partitions.
type TablesMetadataReader interface {
	FindStagesByRoute(context.Context, string, string, string, string) ([]domain.TripDetailsStageMetadata, error)
	FindStagesBySchedule(context.Context, string, string, string) ([]domain.TripDetailsStageMetadata, error)
}

// RouteMetadataLookup identifies one route metadata partition.
type RouteMetadataLookup struct {
	OperatorCode string
	TravelDate   string
	FromCode     string
	ToCode       string
}

// ScheduleMetadataLookup identifies one schedule metadata partition.
type ScheduleMetadataLookup struct {
	OperatorCode string
	ScheduleCode string
	TravelDate   string
}

// TablesService provides read-only Cassandra table data for the protected UI.
type TablesService struct {
	periodicRoutes TablesPeriodicRefreshRouteReader
	metadata       TablesMetadataReader
}

// NewTablesService constructs the protected Tables UI read service.
func NewTablesService(periodicRoutes TablesPeriodicRefreshRouteReader, metadata TablesMetadataReader) *TablesService {
	return &TablesService{periodicRoutes: periodicRoutes, metadata: metadata}
}

// ListPeriodicRefreshRoutes returns every active and inactive scheduled route.
func (service *TablesService) ListPeriodicRefreshRoutes(ctx context.Context) ([]domain.PeriodicRefreshRoute, error) {
	if service == nil || service.periodicRoutes == nil {
		return nil, ErrTablesNotConfigured
	}
	return service.periodicRoutes.ListPeriodicRefreshRoutes(ctx)
}

// FindRouteMetadata returns metadata for a complete operator/travel/from/to key.
func (service *TablesService) FindRouteMetadata(ctx context.Context, lookup RouteMetadataLookup) ([]domain.TripDetailsStageMetadata, error) {
	if service == nil || service.metadata == nil {
		return nil, ErrTablesNotConfigured
	}
	if !validTablesLookup(lookup.OperatorCode, lookup.TravelDate, lookup.FromCode, lookup.ToCode) {
		return nil, ErrInvalidTablesLookup
	}
	return service.metadata.FindStagesByRoute(ctx, lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
}

// FindScheduleMetadata returns metadata for a complete operator/schedule/travel key.
func (service *TablesService) FindScheduleMetadata(ctx context.Context, lookup ScheduleMetadataLookup) ([]domain.TripDetailsStageMetadata, error) {
	if service == nil || service.metadata == nil {
		return nil, ErrTablesNotConfigured
	}
	if !validTablesLookup(lookup.OperatorCode, lookup.ScheduleCode, lookup.TravelDate) {
		return nil, ErrInvalidTablesLookup
	}
	return service.metadata.FindStagesBySchedule(ctx, lookup.OperatorCode, lookup.ScheduleCode, lookup.TravelDate)
}

func validTablesLookup(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.Contains(value, ":") {
			return false
		}
	}
	return true
}
