package http

import (
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// NewRouter builds the Phase 1 HTTP routing for orbitplusmaster.
func NewRouter(tripDetailsService *master.TripDetailsService) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/tripdetails", NewTripDetailsHandler(tripDetailsService))
	mux.Handle("GET /health", NewHealthHandler())
	return mux
}
