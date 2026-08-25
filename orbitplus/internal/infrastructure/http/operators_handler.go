package http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

const operatorRequestMaxBytes = 4096

// OperatorsHandler serves protected operator registry management APIs.
type OperatorsHandler struct {
	registry master.OperatorRegistry
	logger   *log.Logger
}

// NewOperatorsHandler constructs an operator registry API handler.
func NewOperatorsHandler(registry master.OperatorRegistry) *OperatorsHandler {
	return &OperatorsHandler{registry: registry, logger: log.Default()}
}

type operatorItem struct {
	OperatorCode string `json:"operatorCode"`
	ZoneCode     string `json:"zoneCode"`
	Active       bool   `json:"active"`
}

type createOperatorRequest struct {
	OperatorCode string `json:"operatorCode"`
	ZoneCode     string `json:"zoneCode"`
}

type setOperatorStatusRequest struct {
	Active *bool `json:"active"`
}

// ServeList returns all active and inactive operators.
func (handler *OperatorsHandler) ServeList(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	operators, err := handler.registry.ListOperators(request.Context())
	if err != nil {
		handler.writeError(response, "list operators", err)
		return
	}
	items := make([]operatorItem, 0, len(operators))
	for _, operator := range operators {
		items = append(items, newOperatorItem(operator))
	}
	writeJSONData(response, items)
}

// ServeCreate registers one new active operator.
func (handler *OperatorsHandler) ServeCreate(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	var body createOperatorRequest
	if !decodeOperatorRequest(response, request, &body) {
		return
	}
	operator, err := handler.registry.RegisterOperator(request.Context(), body.OperatorCode, body.ZoneCode)
	if err != nil {
		handler.writeError(response, "create operator", err)
		return
	}
	writeJSONData(response, newOperatorItem(operator))
}

// ServeStatus changes an existing operator's active state.
func (handler *OperatorsHandler) ServeStatus(response http.ResponseWriter, request *http.Request) {
	if !handler.available(response) {
		return
	}
	var body setOperatorStatusRequest
	if !decodeOperatorRequest(response, request, &body) || body.Active == nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "active must be provided as true or false")
		return
	}
	operator, err := handler.registry.SetOperatorActive(request.Context(), request.PathValue("operatorCode"), *body.Active)
	if err != nil {
		handler.writeError(response, "set operator status", err)
		return
	}
	writeJSONData(response, newOperatorItem(operator))
}

func (handler *OperatorsHandler) available(response http.ResponseWriter) bool {
	if handler.registry != nil {
		return true
	}
	writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Operator registry is not configured")
	return false
}

func (handler *OperatorsHandler) writeError(response http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, master.ErrInvalidOperatorCode):
		writeJSONStatus(response, http.StatusBadRequest, 0, "operatorCode is required and must be at most 128 characters")
	case errors.Is(err, master.ErrInvalidOperatorZoneCode):
		writeJSONStatus(response, http.StatusBadRequest, 0, "zoneCode is required and is not supported")
	case errors.Is(err, master.ErrOperatorNotFound):
		writeJSONStatus(response, http.StatusNotFound, 0, "Operator not found")
	case errors.Is(err, master.ErrOperatorRegistryUnavailable):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Operator registry is unavailable")
	default:
		handler.logger.Printf("Operator registry %s failed: %v", operation, err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to update operator registry")
	}
}

func decodeOperatorRequest(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, operatorRequestMaxBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid operator request JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid operator request JSON")
		return false
	}
	return true
}

func newOperatorItem(operator domain.Operator) operatorItem {
	return operatorItem{OperatorCode: strings.TrimSpace(operator.Code), ZoneCode: strings.TrimSpace(operator.ZoneCode), Active: operator.Active()}
}
