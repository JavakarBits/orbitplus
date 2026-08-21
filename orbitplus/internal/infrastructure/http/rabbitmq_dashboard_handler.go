package http

import (
	"log"
	"net/http"

	"orbitplusmaster/internal/infrastructure/rabbitmq"
)

// RabbitMQDashboardHandler serves protected, read-only RabbitMQ broker data.
type RabbitMQDashboardHandler struct {
	reader rabbitmq.ManagementReader
	logger *log.Logger
}

// NewRabbitMQDashboardHandler constructs a RabbitMQ Management API handler.
func NewRabbitMQDashboardHandler(reader rabbitmq.ManagementReader) *RabbitMQDashboardHandler {
	return &RabbitMQDashboardHandler{reader: reader, logger: log.Default()}
}

// ServeHTTP returns selected Management API overview, queue, exchange, and connection data.
func (handler *RabbitMQDashboardHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.reader == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "RabbitMQ Management API is not configured")
		return
	}
	snapshot, err := handler.reader.Snapshot(request.Context())
	if err != nil {
		handler.logger.Printf("RabbitMQ management read failed: %v", err)
		writeJSONStatus(response, http.StatusBadGateway, 0, "Unable to load RabbitMQ broker data")
		return
	}
	writeJSONData(response, snapshot)
}
