package http

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"orbitplusmaster/internal/application/master"
)

// CacheHandler serves protected, read-only Dragonfly cache viewer APIs.
type CacheHandler struct {
	service *master.CacheReadService
	logger  *log.Logger
}

// NewCacheHandler constructs a Cache API handler.
func NewCacheHandler(service *master.CacheReadService) *CacheHandler {
	return &CacheHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns a cursor-based page of cache keys without their values.
func (handler *CacheHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache access is not configured")
		return
	}
	cursor, pageSize, err := cacheViewerRequest(request)
	if err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Cache cursor or page size is invalid")
		return
	}
	page, err := handler.service.List(request.Context(), cursor, request.URL.Query().Get("category"), pageSize)
	if err != nil {
		handler.writeError(response, err)
		return
	}
	items := make([]cacheViewerItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, cacheViewerItem{Key: item.Key, Category: item.Category})
	}
	writeJSONData(response, cacheViewerResponse{Items: items, NextCursor: page.NextCursor, PageSize: page.PageSize})
}

// ServeValue returns one cache value after a user selects its key in the viewer.
func (handler *CacheHandler) ServeValue(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache access is not configured")
		return
	}
	value, err := handler.service.Get(request.Context(), request.URL.Query().Get("key"))
	if err != nil {
		handler.writeError(response, err)
		return
	}
	writeJSONData(response, cacheValueResponse{Key: value.Key, Found: value.Found, Content: value.Content})
}

type cacheViewerItem struct {
	Key      string `json:"key"`
	Category string `json:"category"`
}
type cacheViewerResponse struct {
	Items      []cacheViewerItem `json:"items"`
	NextCursor uint64            `json:"nextCursor"`
	PageSize   int64             `json:"pageSize"`
}
type cacheValueResponse struct {
	Key     string `json:"key"`
	Found   bool   `json:"found"`
	Content string `json:"content"`
}

func cacheViewerRequest(request *http.Request) (uint64, int64, error) {
	cursor, err := cacheViewerUint(request.URL.Query().Get("cursor"))
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := cacheViewerInt(request.URL.Query().Get("limit"))
	if err != nil {
		return 0, 0, err
	}
	return cursor, pageSize, nil
}
func cacheViewerUint(value string) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}
func cacheViewerInt(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}
func (handler *CacheHandler) writeError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, master.ErrCacheNotConfigured):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache access is not configured")
	case errors.Is(err, master.ErrInvalidCacheCategory), errors.Is(err, master.ErrInvalidCacheKey):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Cache request is invalid")
	default:
		handler.logger.Printf("Cache viewer read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load cache data")
	}
}
