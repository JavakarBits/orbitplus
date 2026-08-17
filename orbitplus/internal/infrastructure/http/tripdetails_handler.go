package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

// TripDetailsHandler handles POST /api/tripdetails requests. It validates
// that the request body is well-formed JSON and delegates to
// TripDetailsService for the Phase 1 application logic.
type TripDetailsHandler struct {
	service *master.TripDetailsService
}

// NewTripDetailsHandler constructs a TripDetailsHandler.
func NewTripDetailsHandler(service *master.TripDetailsService) *TripDetailsHandler {
	return &TripDetailsHandler{service: service}
}

func (handler *TripDetailsHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}

	var tripDetails domain.TripDetailsResponse
	if err := json.Unmarshal(rawBody, &tripDetails); err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid trip details JSON")
		return
	}

	handler.service.ReceiveTripDetails(rawBody)
	writeJSONStatus(response, http.StatusOK, 1, "Trip details received successfully")
}

func writeJSONStatus(response http.ResponseWriter, httpStatusCode, status int, message string) {
	body, err := json.Marshal(map[string]any{"status": status, "message": message})
	if err != nil {
		log.Printf("failed to marshal response body: %v", err)
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = response.Write([]byte(`{"status":0,"message":"Internal server error"}`))
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(httpStatusCode)
	_, _ = response.Write(body)
}
