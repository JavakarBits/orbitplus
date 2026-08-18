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
	logger  *log.Logger
}

// NewTripDetailsReadHandler constructs a read handler using the process logger.
func NewTripDetailsReadHandler(service *master.TripDetailsReadService) *TripDetailsReadHandler {
	return &TripDetailsReadHandler{service: service, logger: log.Default()}
}

func (handler *TripDetailsReadHandler) ServeSearch(response http.ResponseWriter, request *http.Request) {
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		FromCode:     request.PathValue("fromCode"),
		ToCode:       request.PathValue("toCode"),
		TravelDate:   request.PathValue("tripDate"),
	}
	if !master.ValidSearchLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
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
	lookup := master.RouteLookup{
		OperatorCode: request.PathValue("operatorCode"),
		TripCode:     request.PathValue("tripCode"),
		FromCode:     request.PathValue("fromStationCode"),
		ToCode:       request.PathValue("toStationCode"),
		TravelDate:   request.PathValue("travelDate"),
	}
	if !master.ValidBusMapLookup(lookup) {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Invalid request")
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
