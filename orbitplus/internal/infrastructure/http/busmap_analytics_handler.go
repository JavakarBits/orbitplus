package http

import (
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

// BusmapAnalyticsHandler returns recorded cache-versus-Bits differences for the
// report UI.
type BusmapAnalyticsHandler struct {
	service *master.BusmapAnalyticsService
	logger  *log.Logger
}

// NewBusmapAnalyticsHandler constructs the report API handler.
func NewBusmapAnalyticsHandler(service *master.BusmapAnalyticsService) *BusmapAnalyticsHandler {
	return &BusmapAnalyticsHandler{service: service, logger: log.Default()}
}

// ServeHTTP lists the bounded set of recorded difference records.
func (handler *BusmapAnalyticsHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Busmap data analytics is not configured")
		return
	}
	records, err := handler.service.List(request.Context())
	if err != nil {
		handler.logger.Printf("Busmap data analytics read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load busmap data analytics")
		return
	}
	items := make([]busmapAnalyticsReportItem, 0, len(records))
	for _, record := range records {
		items = append(items, newBusmapAnalyticsReportItem(record))
	}
	writeJSONData(response, items)
}

type busmapAnalyticsReportItem struct {
	DifferenceID        string     `json:"differenceId"`
	OperatorCode        string     `json:"operatorCode"`
	ActionType          string     `json:"actionType"`
	TripCode            string     `json:"tripCode"`
	TripStageCode       string     `json:"tripStageCode"`
	FromCode            string     `json:"fromCode"`
	ToCode              string     `json:"toCode"`
	TripDate            string     `json:"tripDate"`
	VerificationOutcome string     `json:"verificationOutcome"`
	DifferenceCount     int        `json:"differenceCount"`
	DifferencePaths     []string   `json:"differencePaths"`
	CacheRepaired       bool       `json:"cacheRepaired"`
	DetectedAt          *time.Time `json:"detectedAt"`
}

func newBusmapAnalyticsReportItem(record domain.RecordedDifference) busmapAnalyticsReportItem {
	paths := record.DifferencePaths
	if paths == nil {
		paths = []string{}
	}
	return busmapAnalyticsReportItem{
		DifferenceID: record.DifferenceID, OperatorCode: record.OperatorCode, ActionType: record.ActionType,
		TripCode: record.TripCode, TripStageCode: record.TripStageCode, FromCode: record.FromCode,
		ToCode: record.ToCode, TripDate: record.TripDate, VerificationOutcome: string(record.VerificationOutcome),
		DifferenceCount: record.DifferenceCount, DifferencePaths: paths, CacheRepaired: record.CacheRepaired,
		DetectedAt: optionalReportTime(record.DetectedAt),
	}
}
