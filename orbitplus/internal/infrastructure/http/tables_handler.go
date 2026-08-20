package http

import (
	"errors"
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

// TablesHandler serves protected read-only Cassandra tables APIs.
type TablesHandler struct {
	service *master.TablesService
	logger  *log.Logger
}

// NewTablesHandler constructs a Tables API handler.
func NewTablesHandler(service *master.TablesService) *TablesHandler {
	return &TablesHandler{service: service, logger: log.Default()}
}

// ServeRouteMetadata returns metadata for a required route partition.
func (handler *TablesHandler) ServeRouteMetadata(response http.ResponseWriter, request *http.Request) {
	lookup := master.RouteMetadataLookup{
		OperatorCode: request.URL.Query().Get("operator"),
		TravelDate:   request.URL.Query().Get("travel"),
		FromCode:     request.URL.Query().Get("from"),
		ToCode:       request.URL.Query().Get("to"),
	}
	handler.serveMetadata(response, "route metadata", func() ([]domain.TripDetailsStageMetadata, error) {
		return handler.service.FindRouteMetadata(request.Context(), lookup)
	})
}

// ServeScheduleMetadata returns metadata for a required schedule partition.
func (handler *TablesHandler) ServeScheduleMetadata(response http.ResponseWriter, request *http.Request) {
	lookup := master.ScheduleMetadataLookup{
		OperatorCode: request.URL.Query().Get("operator"),
		ScheduleCode: request.URL.Query().Get("schedule"),
		TravelDate:   request.URL.Query().Get("travel"),
	}
	handler.serveMetadata(response, "schedule metadata", func() ([]domain.TripDetailsStageMetadata, error) {
		return handler.service.FindScheduleMetadata(request.Context(), lookup)
	})
}

func (handler *TablesHandler) serveMetadata(response http.ResponseWriter, operation string, read func() ([]domain.TripDetailsStageMetadata, error)) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Tables reporting is not configured")
		return
	}
	metadata, err := read()
	if err != nil {
		handler.writeError(response, operation, err)
		return
	}
	items := make([]metadataTableItem, 0, len(metadata))
	for _, item := range metadata {
		items = append(items, newMetadataTableItem(item))
	}
	writeJSONData(response, items)
}

func (handler *TablesHandler) writeError(response http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, master.ErrTablesNotConfigured):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Tables reporting is not configured")
	case errors.Is(err, master.ErrInvalidTablesLookup):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Required lookup values are missing or invalid")
	default:
		handler.logger.Printf("Tables %s read failed: %v", operation, err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load table data")
	}
}

type metadataTableItem struct {
	OperatorCode  string     `json:"operatorCode"`
	ScheduleCode  string     `json:"scheduleCode"`
	TravelDate    string     `json:"travelDate"`
	FromStation   string     `json:"fromStation"`
	ToStation     string     `json:"toStation"`
	TripCode      string     `json:"tripCode"`
	TripStageCode string     `json:"tripStageCode"`
	UpdatedAt     *time.Time `json:"updatedAt"`
}

func newMetadataTableItem(metadata domain.TripDetailsStageMetadata) metadataTableItem {
	return metadataTableItem{OperatorCode: metadata.OperatorCode, ScheduleCode: metadata.ScheduleCode,
		TravelDate: metadata.TravelDate, FromStation: metadata.FromStationCode, ToStation: metadata.ToStationCode,
		TripCode: metadata.TripCode, TripStageCode: metadata.TripStageCode, UpdatedAt: optionalTablesTime(metadata.UpdatedAt)}
}

func optionalTablesTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
