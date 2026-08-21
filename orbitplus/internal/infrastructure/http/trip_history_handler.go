package http

import (
	"errors"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// TripHistoryHandler serves the protected queue lifecycle history of one trip.
type TripHistoryHandler struct {
	service *master.TripHistoryService
	logger  *log.Logger
}

// NewTripHistoryHandler constructs the trip analyzer API handler.
func NewTripHistoryHandler(service *master.TripHistoryService) *TripHistoryHandler {
	return &TripHistoryHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns queue_metrix lifecycle records for one operator code and trip code.
func (handler *TripHistoryHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Trip history reporting is not configured")
		return
	}
	parameters := request.URL.Query()
	summary, err := handler.service.Lookup(request.Context(), master.TripHistoryQuery{
		OperatorCode: parameters.Get("operatorCode"),
		TripCode:     parameters.Get("tripCode"),
		TripDate:     parameters.Get("tripDate"),
		FromStation:  parameters.Get("fromStation"),
		ToStation:    parameters.Get("toStation"),
	})
	if errors.Is(err, master.ErrInvalidTripHistoryLookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "operatorCode is required, together with a tripCode or at least one of tripDate, fromStation, or toStation")
		return
	}
	if errors.Is(err, master.ErrTripHistoryNotConfigured) {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Trip history reporting is not configured")
		return
	}
	if errors.Is(err, master.ErrTripHistoryBusy) {
		writeJSONStatus(response, http.StatusTooManyRequests, 0, "Another trip lookup is already running. This search scans queue_metrix, so only one runs at a time. Try again shortly.")
		return
	}
	if err != nil {
		handler.logger.Printf("Trip history read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load trip history")
		return
	}
	writeJSONData(response, summary)
}
