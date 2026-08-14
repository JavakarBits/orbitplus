package worker

import (
	"log/slog"
	"net/http"
	"sync/atomic"
)

// Readiness tracks whether this process is accepting RabbitMQ deliveries.
type Readiness struct {
	ready atomic.Bool
}

func NewReadiness() *Readiness {
	return &Readiness{}
}

func (readiness *Readiness) MarkReady() {
	if readiness.ready.CompareAndSwap(false, true) {
		slog.Info("tripdetails refresh worker marked ready")
	}
}

func (readiness *Readiness) MarkNotReady() {
	if readiness.ready.CompareAndSwap(true, false) {
		slog.Info("tripdetails refresh worker marked not ready")
	}
}

func (readiness *Readiness) IsReady() bool {
	return readiness != nil && readiness.ready.Load()
}

// NewHealthHandler provides dependency-independent liveness and readiness endpoints.
func NewHealthHandler(readiness *Readiness) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(response http.ResponseWriter, _ *http.Request) {
		writeHealthStatus(response, http.StatusOK, "UP")
	})
	mux.HandleFunc("GET /ready", func(response http.ResponseWriter, _ *http.Request) {
		if readiness.IsReady() {
			writeHealthStatus(response, http.StatusOK, "READY")
			return
		}
		writeHealthStatus(response, http.StatusServiceUnavailable, "NOT_READY")
	})
	return mux
}

func writeHealthStatus(response http.ResponseWriter, statusCode int, status string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(statusCode)
	_, _ = response.Write([]byte(`{"status":"` + status + `"}`))
}
