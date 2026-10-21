package http

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"orbitplusmaster/internal/application/master"
)

// TopRoutesHandler serves the protected, read-only Top Routes report API: the
// most-requested from-station -> to-station pairs within a trip-date range.
type TopRoutesHandler struct {
	service *master.TopRoutesService
	logger  *log.Logger
}

// NewTopRoutesHandler constructs a Top Routes report API handler.
func NewTopRoutesHandler(service *master.TopRoutesService) *TopRoutesHandler {
	return &TopRoutesHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns the ranked routes for ?from=YYYY-MM-DD&to=YYYY-MM-DD, with
// an optional ?limit=.
func (handler *TopRoutesHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Top routes report is not configured")
		return
	}
	query := request.URL.Query()
	limit := 0
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeJSONStatus(response, http.StatusBadRequest, 0, "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}
	report, err := handler.service.Report(request.Context(), query.Get("from"), query.Get("to"), limit)
	if err != nil {
		switch {
		case errors.Is(err, master.ErrTopRoutesNotConfigured):
			writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Top routes report is not configured")
		case errors.Is(err, master.ErrInvalidTopRoutesRange):
			writeJSONStatus(response, http.StatusBadRequest, 0, "Provide from and to as YYYY-MM-DD, with from on or before to")
		default:
			handler.logger.Printf("top routes report failed: %v", err)
			writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to build the top routes report")
		}
		return
	}
	writeJSONData(response, report)
}
