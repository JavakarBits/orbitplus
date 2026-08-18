package http

import (
	"io"
	"log"
	"net/http"
	"strings"

	"orbitplusmaster/internal/application/master"
)

// OrionmaxInventoryChangeHandler handles POST /api/orionmax/inventory/events requests.
type OrionmaxInventoryChangeHandler struct {
	service *master.OrionmaxInventoryEventService
	logger  *log.Logger
}

// NewOrionmaxInventoryChangeHandler constructs an inventory-change HTTP handler.
func NewOrionmaxInventoryChangeHandler(service *master.OrionmaxInventoryEventService) *OrionmaxInventoryChangeHandler {
	return &OrionmaxInventoryChangeHandler{service: service, logger: log.Default()}
}

// ServeHTTP accepts one valid JSON inventory-change event with an activity_type query parameter.
func (handler *OrionmaxInventoryChangeHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	activityType := strings.TrimSpace(request.URL.Query().Get("activity_type"))
	if activityType == "" {
		writeJSONStatus(response, http.StatusBadRequest, 0, "activity_type is required")
		return
	}
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		handler.logger.Print("Orionmax inventory change request body read failed")
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	if _, err := decodeSingleJSONValue(rawBody); err != nil {
		handler.logger.Print("Orionmax inventory change JSON validation failed")
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid inventory change JSON")
		return
	}
	if handler.service == nil || handler.service.ReceiveInventoryChange(activityType, rawBody) != nil {
		handler.logger.Print("Orionmax inventory change processing failed")
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	writeJSONStatus(response, http.StatusOK, 1, "Orionmax inventory change received successfully")
}
