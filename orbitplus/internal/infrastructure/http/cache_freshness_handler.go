package http

import (
	"errors"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// CacheFreshnessHandler serves the protected, read-only cache-freshness count
// API used by the admin "Cache freshness" page.
type CacheFreshnessHandler struct {
	service *master.CacheFreshnessCountService
	logger  *log.Logger
}

// NewCacheFreshnessHandler constructs a cache-freshness API handler.
func NewCacheFreshnessHandler(service *master.CacheFreshnessCountService) *CacheFreshnessHandler {
	return &CacheFreshnessHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns the current Hot/Warm/Cold/Total snapshot as JSON.
func (handler *CacheFreshnessHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache freshness count is not configured")
		return
	}
	counts, err := handler.service.Counts(request.Context())
	if err != nil {
		if errors.Is(err, master.ErrCacheFreshnessNotConfigured) {
			writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache freshness count is not configured")
			return
		}
		handler.logger.Printf("cache freshness count failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load cache freshness counts")
		return
	}
	writeJSONData(response, counts)
}

// ServeEntries returns the list of trip stages in one freshness tier, selected
// by the required ?tier=hot|warm|cold query parameter.
func (handler *CacheFreshnessHandler) ServeEntries(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache freshness count is not configured")
		return
	}
	tier, ok := master.ParseFreshnessTier(request.URL.Query().Get("tier"))
	if !ok {
		writeJSONStatus(response, http.StatusBadRequest, 0, "tier must be one of hot, warm, cold")
		return
	}
	entries, err := handler.service.Entries(request.Context(), tier)
	if err != nil {
		if errors.Is(err, master.ErrCacheFreshnessNotConfigured) {
			writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache freshness count is not configured")
			return
		}
		handler.logger.Printf("cache freshness entries failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load cache freshness entries")
		return
	}
	writeJSONData(response, cacheFreshnessEntriesResponse{Entries: entries, Count: len(entries)})
}

type cacheFreshnessEntriesResponse struct {
	Entries []master.CacheFreshnessEntry `json:"entries"`
	Count   int                          `json:"count"`
}
