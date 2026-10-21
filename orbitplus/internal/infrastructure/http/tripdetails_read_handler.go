package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	// credentialReader resolves the operator's stored BITS zone URL so a live
	// read targets the same host the operator was registered with, rather than
	// a hardcoded zone table. It is optional; when nil the handler falls back to
	// the zone-code table.
	credentialReader OperatorCredentialReader
	searchLogger     *log.Logger
	busmapLogger     *log.Logger
}

// NewTripDetailsReadHandler constructs a read handler with dedicated loggers
// for Search and BusMap calls that write to separate log files.
func NewTripDetailsReadHandler(service *master.TripDetailsReadService, verifier *master.CacheFreshnessVerifier, credentialReader OperatorCredentialReader, searchLogger *log.Logger, busmapLogger *log.Logger) *TripDetailsReadHandler {
	if searchLogger == nil {
		searchLogger = log.Default()
	}
	if busmapLogger == nil {
		busmapLogger = log.Default()
	}
	return &TripDetailsReadHandler{service: service, verifier: verifier, credentialReader: credentialReader, searchLogger: searchLogger, busmapLogger: busmapLogger}
}

// resolveLiveBaseURL returns the BITS base URL for a live read. It prefers the
// operator's stored zone URL (resolved operator-wise from Orbit and persisted),
// so nothing is hardcoded per deployment. It falls back to the zone-code table
// only when no stored URL is available, preserving the previous behaviour.
func (handler *TripDetailsReadHandler) resolveLiveBaseURL(ctx context.Context, operatorCode, zoneCode string, callLogger *log.Logger) (string, bool) {
	if handler.credentialReader != nil {
		credential, found, err := handler.credentialReader.FindOperatorCredential(ctx, operatorCode)
		switch {
		case err != nil:
			callLogger.Printf("live read: operator zone URL lookup failed; falling back to zone table operator=%q error=%v",
				operatorCode, err)
		case found && strings.TrimSpace(credential.ZoneURL) != "":
			return strings.TrimRight(strings.TrimSpace(credential.ZoneURL), "/"), true
		default:
			callLogger.Printf("live read: no stored zone URL for operator; falling back to zone table operator=%q",
				operatorCode)
		}
	}
	return master.ZoneBitsBaseURL(zoneCode)
}

func (handler *TripDetailsReadHandler) ServeSearch(response http.ResponseWriter, request *http.Request) {
	cacheFlag, flagErr := parseCacheFlag(request.URL.RawQuery)
	zoneCode, zoneErr := parseZoneCode(request.URL.RawQuery)
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		FromCode:     request.PathValue("fromCode"),
		ToCode:       request.PathValue("toCode"),
		TripDate:     request.PathValue("tripDate"),
		// Carried for a Cold cache refresh (cacheFlag=1 reads that find stale
		// data fetch BITS using these path credentials). Unused by Hot/Warm.
		Username: request.PathValue("username"),
		APIToken: request.PathValue("apiToken"),
	}
	// One combined rejection so the response cannot depend on which of the
	// validations happens to run first.
	if flagErr != nil || !master.ValidSearchLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
		return
	}
	if cacheFlag == cacheFlagFromLive {
		username := request.PathValue("username")
		apiToken := request.PathValue("apiToken")
		baseURL, resolved := handler.resolveLiveBaseURL(request.Context(), lookup.OperatorCode, zoneCode, handler.searchLogger)
		if zoneErr != nil || !resolved || !master.ValidLiveSearchLookup(lookup, username, apiToken) {
			// An unknown or absent zone, or missing path credentials, is the
			// caller's error, not an upstream failure.
			handler.searchLogger.Printf("live read rejected: action=SEARCH operator=%q reason=invalid_live_request",
				lookup.OperatorCode)
			writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
			return
		}
		handler.serveLive(response, request, master.BitsLookup{
			Action:       master.BitsActionSearch,
			OperatorCode: lookup.OperatorCode,
			BaseURL:      baseURL,
			Username:     username,
			APIToken:     apiToken,
			FromCode:     lookup.FromCode,
			ToCode:       lookup.ToCode,
			TravelDate:   lookup.TripDate,
		})
		return
	}
	if handler.service == nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	handler.searchLogger.Printf("search read started: operator=%q from=%q to=%q date=%q",
		lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TripDate)
	data, err := handler.service.Search(request.Context(), lookup)
	if err != nil {
		handler.writeSearchError(response, err)
		return
	}
	handler.searchLogger.Printf("search read completed: operator=%q from=%q to=%q date=%q results=%d",
		lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TripDate, len(data))
	writeJSONData(response, data)
}

func (handler *TripDetailsReadHandler) ServeBusMap(response http.ResponseWriter, request *http.Request) {
	cacheFlag, flagErr := parseCacheFlag(request.URL.RawQuery)
	zoneCode, zoneErr := parseZoneCode(request.URL.RawQuery)
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		TripCode:     request.PathValue("tripCode"),
		FromCode:     request.PathValue("fromStationCode"),
		ToCode:       request.PathValue("toStationCode"),
		TripDate:     request.PathValue("travelDate"),
		// Carried for a Cold cache refresh. Unused by Hot/Warm.
		Username: request.PathValue("username"),
		APIToken: request.PathValue("apiToken"),
	}
	// One combined rejection so the response cannot depend on which of the
	// validations happens to run first.
	if flagErr != nil || !master.ValidBusMapLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
		return
	}
	if cacheFlag == cacheFlagFromLive {
		username := request.PathValue("username")
		apiToken := request.PathValue("apiToken")
		baseURL, resolved := handler.resolveLiveBaseURL(request.Context(), lookup.OperatorCode, zoneCode, handler.busmapLogger)
		if zoneErr != nil || !resolved || !master.ValidLiveBusMapLookup(lookup, username, apiToken) {
			handler.busmapLogger.Printf("live read rejected: action=BUSMAP operator=%q reason=invalid_live_request",
				lookup.OperatorCode)
			writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
			return
		}
		handler.serveLive(response, request, master.BitsLookup{
			Action:       master.BitsActionBusMap,
			OperatorCode: lookup.OperatorCode,
			BaseURL:      baseURL,
			Username:     username,
			APIToken:     apiToken,
			TripCode:     lookup.TripCode,
			FromCode:     lookup.FromCode,
			ToCode:       lookup.ToCode,
			TravelDate:   lookup.TripDate,
		})
		return
	}
	if handler.service == nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	handler.busmapLogger.Printf("busmap read started: operator=%q trip=%q from=%q to=%q date=%q",
		lookup.OperatorCode, lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TripDate)
	data, err := handler.service.BusMap(request.Context(), lookup)
	if err != nil {
		handler.writeBusmapError(response, err)
		return
	}
	handler.busmapLogger.Printf("busmap read completed: operator=%q trip=%q from=%q to=%q date=%q bytes=%d",
		lookup.OperatorCode, lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TripDate, len(data))
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
	callLogger := handler.searchLogger
	if lookup.Action == master.BitsActionBusMap {
		callLogger = handler.busmapLogger
	}
	if handler.verifier == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Live verification is not configured")
		return
	}
	result, err := handler.verifier.Verify(request.Context(), lookup, request.RemoteAddr)
	var rateLimitErr *master.ZoneRateLimitError
	switch {
	case errors.As(err, &rateLimitErr):
		retryAfter := int64(math.Ceil(rateLimitErr.RetryAfter.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		response.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
		writeJSONStatus(response, http.StatusTooManyRequests, 0, "BITS zone rate limit exceeded")
	case errors.Is(err, master.ErrVerificationBusy):
		writeJSONStatus(response, http.StatusTooManyRequests, 0, "Live verification busy")
	case errors.Is(err, master.ErrOperatorCredentialUnavailable):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
	case errors.Is(err, master.ErrLiveSourceRejected):
		// Bits is healthy and answered; it just has nothing for this lookup,
		// whether because the date has passed or the operator does not serve the
		// route. That is the same answer the cached path gives when it finds
		// nothing, so it gets the same status rather than a gateway error the
		// caller would retry. The specific code is logged, never returned.
		callLogger.Printf("live read: trip details not found action=%s operator=%q reason=source_rejected",
			lookup.Action, lookup.OperatorCode)
		writeJSONStatus(response, http.StatusNotFound, 0, "Trip details not found")
	case err != nil:
		// The specific reason is logged, never returned, so no upstream status
		// code or body can reach the caller.
		writeJSONStatus(response, http.StatusBadGateway, 0, "Live source unavailable")
	case result.DataEmpty:
		callLogger.Printf("live read: trip details not found action=%s operator=%q reason=empty_data",
			lookup.Action, lookup.OperatorCode)
		writeJSONStatus(response, http.StatusNotFound, 0, "Trip details not found")
	default:
		// Reusing writeJSONData keeps the envelope, member order, datetime
		// rendering, and Content-Type identical to the cached path by sharing
		// the code rather than by copying it.
		writeJSONData(response, result.Data)
	}
}

func (handler *TripDetailsReadHandler) writeSearchError(response http.ResponseWriter, err error) {
	handler.writeReadError(response, handler.searchLogger, "search", err)
}

func (handler *TripDetailsReadHandler) writeBusmapError(response http.ResponseWriter, err error) {
	handler.writeReadError(response, handler.busmapLogger, "busmap", err)
}

func (handler *TripDetailsReadHandler) writeReadError(response http.ResponseWriter, callLogger *log.Logger, operation string, err error) {
	if errors.Is(err, master.ErrTripDetailsNotFound) {
		callLogger.Printf("%s read: trip details not found", operation)
		writeJSONStatus(response, http.StatusNotFound, 0, "Trip details not found")
		return
	}
	callLogger.Printf("TripDetails %s read failed: %v", operation, err)
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
