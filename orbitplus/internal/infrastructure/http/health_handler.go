package http

import (
	"encoding/json"
	"net/http"
)

// NewHealthHandler returns a handler for GET /health that reports basic
// liveness and the running service version. Phase 1 has no dependencies to
// check, so this always reports UP.
func NewHealthHandler(version string) http.Handler {
	body, _ := json.Marshal(map[string]string{"status": "UP", "version": version})
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(body)
	})
}
