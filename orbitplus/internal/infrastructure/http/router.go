package http

import (
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// NewRouter builds HTTP routing for incoming inventory changes, health, and persisted read APIs.
func NewRouter(tripDetailsService *master.TripDetailsService, orionmaxInventoryChangeService *master.OrionmaxInventoryEventService, readServices ...*master.TripDetailsReadService) http.Handler {
	var readService *master.TripDetailsReadService
	if len(readServices) > 0 {
		readService = readServices[0]
	}
	readHandler := NewTripDetailsReadHandler(readService)
	mux := http.NewServeMux()
	mux.Handle("POST /api/tripdetails", NewTripDetailsHandler(tripDetailsService))
	mux.Handle("POST /api/orionmax/inventory/events", NewOrionmaxInventoryChangeHandler(orionmaxInventoryChangeService))
	mux.Handle("GET /health", NewHealthHandler())
	mux.Handle("GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}", http.HandlerFunc(readHandler.ServeSearch))
	mux.Handle("GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}", http.HandlerFunc(readHandler.ServeBusMap))
	mux.Handle("GET /orbitplus/api/3.0/json/", http.HandlerFunc(readHandler.ServeInvalidRoute))
	return mux
}
