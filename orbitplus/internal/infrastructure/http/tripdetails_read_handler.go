package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
)

// TripDetailsReadHandler serves persisted BusIQ Search and BUSMAP responses.
type TripDetailsReadHandler struct {
	service *master.TripDetailsReadService
	// verifier is nil when live verification is unconfigured, in which case a
	// Cache_Flag value of 0 reports the feature as unavailable rather than
	// silently serving the cached copy.
	verifier *master.CacheFreshnessVerifier
	logger   *log.Logger
}

// NewTripDetailsReadHandler constructs a read handler using the process logger.
// A nil verifier disables the live path and leaves the cached path untouched.
func NewTripDetailsReadHandler(service *master.TripDetailsReadService, verifier *master.CacheFreshnessVerifier) *TripDetailsReadHandler {
	return &TripDetailsReadHandler{service: service, verifier: verifier, logger: log.Default()}
}

func (handler *TripDetailsReadHandler) ServeSearch(response http.ResponseWriter, request *http.Request) {
	cacheFlag, flagErr := parseCacheFlag(request.URL.RawQuery)
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		FromCode:     request.PathValue("fromCode"),
		ToCode:       request.PathValue("toCode"),
		TripDate:     request.PathValue("tripDate"),
	}
	// One combined rejection so the response cannot depend on which of the two
	// validations happens to run first.
	if flagErr != nil || !master.ValidSearchLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
		return
	}
	if cacheFlag == cacheFlagFromLive {
		handler.serveLive(response, request, master.BitsLookup{
			Action:       master.BitsActionSearch,
			OperatorCode: lookup.OperatorCode,
			FromCode:     lookup.FromCode,
			ToCode:       lookup.ToCode,
			TravelDate:   lookup.TravelDate,
		})
		return
	}
	if handler.service == nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	data, err := handler.service.Search(request.Context(), lookup)
	if err != nil {
		handler.writeReadError(response, "search", err)
		return
	}
	writeJSONData(response, data)
}

func (handler *TripDetailsReadHandler) ServeBusMap(response http.ResponseWriter, request *http.Request) {
	cacheFlag, flagErr := parseCacheFlag(request.URL.RawQuery)
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		TripCode:     request.PathValue("tripCode"),
		FromCode:     request.PathValue("fromStationCode"),
		ToCode:       request.PathValue("toStationCode"),
		TripDate:     request.PathValue("travelDate"),
	}
	// One combined rejection so the response cannot depend on which of the two
	// validations happens to run first.
	if flagErr != nil || !master.ValidBusMapLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
		return
	}
	if cacheFlag == cacheFlagFromLive {
		handler.serveLive(response, request, master.BitsLookup{
			Action:       master.BitsActionBusMap,
			OperatorCode: lookup.OperatorCode,
			TripCode:     lookup.TripCode,
			FromCode:     lookup.FromCode,
			ToCode:       lookup.ToCode,
			TravelDate:   lookup.TravelDate,
		})
		return
	}
	if handler.service == nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	data, err := handler.service.BusMap(request.Context(), lookup)
	if err != nil {
		handler.writeReadError(response, "busmap", err)
		return
	}
	writeJSONData(response, data)
}

func (handler *TripDetailsReadHandler) ServeInvalidRoute(response http.ResponseWriter, _ *http.Request) {
	writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
}

// serveLive answers a Cache_Flag value of 0 from Bits.
//
// There is deliberately no fallback to the cached copy on failure: a caller that
// asked for live data must never receive cache data labelled as live, which is
// the confusion this feature exists to remove.
func (handler *TripDetailsReadHandler) serveLive(response http.ResponseWriter, request *http.Request, lookup master.BitsLookup) {
	if handler.verifier == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Live verification is not configured")
		return
	}
	result, err := handler.verifier.Verify(request.Context(), lookup, request.RemoteAddr)
	switch {
	case errors.Is(err, master.ErrVerificationBusy):
		writeJSONStatus(response, http.StatusTooManyRequests, 0, "Live verification busy")
	case err != nil:
		// The specific reason is logged, never returned, so no upstream status
		// code or body can reach the caller.
		writeJSONStatus(response, http.StatusBadGateway, 0, "Live source unavailable")
	case result.DataEmpty:
		writeJSONStatus(response, http.StatusNotFound, 0, "Trip details not found")
	default:
		// Reusing writeJSONData keeps the envelope, member order, datetime
		// rendering, and Content-Type identical to the cached path by sharing
		// the code rather than by copying it.
		writeJSONData(response, result.Data)
	}
}

func (handler *TripDetailsReadHandler) writeReadError(response http.ResponseWriter, operation string, err error) {
	if errors.Is(err, master.ErrTripDetailsNotFound) {
		writeJSONStatus(response, http.StatusNotFound, 0, "Trip details not found")
		return
	}
	handler.logger.Printf("TripDetails %s read failed", operation)
	writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
}

func writeJSONData(response http.ResponseWriter, data any) {
	body, err := json.Marshal(map[string]any{
		"status":   1,
		"datetime": time.Now().Format("2006-01-02 15:04:05"),
		"data":     data,
	})
	if err != nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(body)
}
