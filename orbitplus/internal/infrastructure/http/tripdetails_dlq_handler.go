package http

import (
	"io"
	"log"
	"net/http"
	"strings"

	"orbitplusmaster/internal/application/master"
)

// TripDetailsDLQHandler records Worker dead-letter notifications.
type TripDetailsDLQHandler struct {
	service *master.TripDetailsService
	logger  *log.Logger
}

// NewTripDetailsDLQHandler constructs a dead-letter notification handler.
func NewTripDetailsDLQHandler(service *master.TripDetailsService) *TripDetailsDLQHandler {
	return &TripDetailsDLQHandler{service: service, logger: log.Default()}
}

// ServeHTTP handles POST /orbitplus/api/tripdetails/dlq.
func (handler *TripDetailsDLQHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid trip details DLQ JSON")
		return
	}
	value, err := decodeSingleJSONValue(rawBody)
	if err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid trip details DLQ JSON")
		return
	}
	payload, ok := value.(map[string]any)
	if !ok {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid trip details DLQ JSON")
		return
	}
	referenceID, _ := payload["referenceId"].(string)
	if referenceID = strings.TrimSpace(referenceID); referenceID == "" {
		writeJSONStatus(response, http.StatusBadRequest, 0, "referenceId is required")
		return
	}
	failureMessage, _ := payload["failureMessage"].(string)
	if failureMessage = strings.TrimSpace(failureMessage); failureMessage == "" {
		writeJSONStatus(response, http.StatusBadRequest, 0, "failureMessage is required")
		return
	}
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Queue lifecycle tracking is not configured")
		return
	}
	if err := handler.service.MarkTripDetailsDead(request.Context(), referenceID, failureMessage); err != nil {
		handler.logger.Printf("TripDetails DLQ notification failed: reference_id=%q error=%v", referenceID, err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	writeJSONStatus(response, http.StatusOK, 1, "Trip details dead-lettered successfully")
}
