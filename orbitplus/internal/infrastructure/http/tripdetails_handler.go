package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// TripDetailsHandler handles POST /api/tripdetails requests and delegates each
// complete, syntactically valid JSON value to TripDetailsService.
type TripDetailsHandler struct {
	service *master.TripDetailsService
	logger  *log.Logger
}

// NewTripDetailsHandler constructs a TripDetailsHandler using the process
// logger.
func NewTripDetailsHandler(service *master.TripDetailsService) *TripDetailsHandler {
	return newTripDetailsHandler(service, log.Default())
}

func newTripDetailsHandler(service *master.TripDetailsService, logger *log.Logger) *TripDetailsHandler {
	return &TripDetailsHandler{service: service, logger: logger}
}

func (handler *TripDetailsHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		handler.logger.Print("TripDetails request body read failed")
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}

	value, err := decodeSingleJSONValue(rawBody)
	if err != nil {
		handler.logger.Print("TripDetails JSON validation failed")
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid trip details JSON")
		return
	}

	if err := handler.service.ReceiveTripDetails(rawBody, value); err != nil {
		handler.logger.Print("TripDetails persistence failed")
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Internal server error")
		return
	}
	writeJSONStatus(response, http.StatusOK, 1, "Trip details received successfully")
}

func decodeSingleJSONValue(rawBody []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	var trailingValue any
	if err := decoder.Decode(&trailingValue); err != io.EOF {
		if err == nil {
			return nil, errMultipleJSONValues
		}
		return nil, err
	}
	return value, nil
}

var errMultipleJSONValues = &jsonValueError{"request body contains more than one JSON value"}

type jsonValueError struct {
	message string
}

func (err *jsonValueError) Error() string {
	return err.message
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
