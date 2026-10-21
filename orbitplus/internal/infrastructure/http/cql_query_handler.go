package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// CqlQueryHandler serves the protected, read-only (SELECT-only) CQL query API
// used by the admin "CQL query" page.
type CqlQueryHandler struct {
	service *master.CqlQueryService
	logger  *log.Logger
}

// NewCqlQueryHandler constructs a CQL query API handler.
func NewCqlQueryHandler(service *master.CqlQueryService) *CqlQueryHandler {
	return &CqlQueryHandler{service: service, logger: log.Default()}
}

type cqlQueryRequest struct {
	Query string `json:"query"`
}

type cqlQueryResponse struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	Truncated bool             `json:"truncated"`
	MaxRows   int              `json:"maxRows"`
}

// ServeHTTP accepts {"query": "SELECT ..."} and returns the result rows. Only a
// single SELECT statement is allowed; anything else is rejected with 400.
func (handler *CqlQueryHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "CQL query is not configured")
		return
	}
	var payload cqlQueryRequest
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Request body must be JSON with a 'query' field")
		return
	}
	result, err := handler.service.Run(request.Context(), payload.Query)
	if err != nil {
		handler.writeError(response, err)
		return
	}
	writeJSONData(response, cqlQueryResponse{
		Columns:   result.Columns,
		Rows:      result.Rows,
		RowCount:  len(result.Rows),
		Truncated: result.Truncated,
		MaxRows:   result.MaxRows,
	})
}

func (handler *CqlQueryHandler) writeError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, master.ErrCqlNotConfigured):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "CQL query is not configured")
	case errors.Is(err, master.ErrCqlEmpty):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Enter a SELECT query")
	case errors.Is(err, master.ErrCqlNotSelect):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Only a single read-only SELECT statement is allowed")
	default:
		handler.logger.Printf("CQL query failed: %v", err)
		writeJSONStatus(response, http.StatusBadRequest, 0, "Query could not be executed; check the statement and table/column names")
	}
}
