package http

import "net/http"

// NewHealthHandler returns a handler for GET /health that reports basic
// liveness. Phase 1 has no dependencies to check, so this always reports UP.
func NewHealthHandler() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"status":"UP"}`))
	})
}
