package http

import (
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/rabbitmq"
)

// AdminDashboardHandler returns a truthful, protected operational snapshot for the admin portal.
type AdminDashboardHandler struct {
	systemHealth  *SystemHealthHandler
	queueJobs     *master.QueueJobsService
	tripFreshness *master.TripFreshnessService
	rabbitMQ      rabbitmq.ManagementReader
	logger        *log.Logger
}

// NewAdminDashboardHandler constructs the aggregate dashboard API handler.
func NewAdminDashboardHandler(systemHealth *SystemHealthHandler, queueJobs *master.QueueJobsService, tripFreshness *master.TripFreshnessService, rabbitMQ rabbitmq.ManagementReader) *AdminDashboardHandler {
	return &AdminDashboardHandler{systemHealth: systemHealth, queueJobs: queueJobs, tripFreshness: tripFreshness, rabbitMQ: rabbitMQ, logger: log.Default()}
}

type adminCapability struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
	Data   any    `json:"data,omitempty"`
}

type adminDashboardResponse struct {
	ObservedAt           time.Time       `json:"observedAt"`
	Runtime              adminCapability `json:"runtime"`
	QueueMetrics         adminCapability `json:"queueMetrics"`
	RabbitMQ             adminCapability `json:"rabbitmq"`
	TripDetailsFreshness adminCapability `json:"tripDetailsFreshness"`
	Workers              adminCapability `json:"workers"`
	Scheduler            adminCapability `json:"scheduler"`
	PriorityDistribution adminCapability `json:"priorityDistribution"`
	DragonflyMetrics     adminCapability `json:"dragonflyMetrics"`
	CassandraMetrics     adminCapability `json:"cassandraMetrics"`
	APIMonitoring        adminCapability `json:"apiMonitoring"`
	Alerts               adminCapability `json:"alerts"`
	InventoryEvents      adminCapability `json:"inventoryEvents"`
}

// ServeHTTP returns each integration independently so an unavailable integration never fabricates or hides other data.
func (handler *AdminDashboardHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	now := time.Now().UTC()
	dashboard := adminDashboardResponse{ObservedAt: now, Runtime: handler.runtime(now), QueueMetrics: handler.queueMetrics(request), RabbitMQ: handler.rabbitMQSnapshot(request)}
	dashboard.TripDetailsFreshness = handler.tripFreshnessSummary(request)
	dashboard.Workers = unavailableCapability("Worker or Kubernetes health telemetry is not configured.")
	dashboard.Scheduler = unavailableCapability("The periodic refresh scheduler has been removed and has no execution telemetry.")
	dashboard.PriorityDistribution = unavailableCapability("Priority-queue telemetry is not configured.")
	dashboard.DragonflyMetrics = unavailableCapability("Dragonfly capacity and latency telemetry is not configured.")
	dashboard.CassandraMetrics = unavailableCapability("Cassandra capacity and latency telemetry is not configured.")
	dashboard.APIMonitoring = unavailableCapability("Request instrumentation and API performance telemetry are not configured.")
	dashboard.Alerts = unavailableCapability("No alerting source is configured.")
	dashboard.InventoryEvents = unavailableCapability("Inventory-event telemetry is not implemented; Queue Metrics may include related lifecycle records.")
	writeJSONData(response, dashboard)
}

func (handler *AdminDashboardHandler) runtime(now time.Time) adminCapability {
	if handler.systemHealth == nil {
		return unavailableCapability("Master runtime telemetry is unavailable.")
	}
	return adminCapability{Status: "available", Detail: "Live Go runtime snapshot.", Data: handler.systemHealth.snapshot(now)}
}

func (handler *AdminDashboardHandler) queueMetrics(request *http.Request) adminCapability {
	if handler.queueJobs == nil {
		return adminCapability{Status: "not_configured", Detail: "Queue Metrics storage is not configured."}
	}
	summary, err := handler.queueJobs.Summary(request.Context())
	if err != nil {
		handler.logger.Printf("Admin dashboard Queue Metrics read failed: %v", err)
		return adminCapability{Status: "source_unavailable", Detail: "Queue Metrics data is currently unavailable."}
	}
	data := queueDashboardResponse{Scope: "Queue Metrics updates from the last 24 hours within the loaded sample (maximum 100)", LoadedRecords: summary.LoadedRecords, Received: summary.Received, Queued: summary.Queued, Completed: summary.Completed, Dead: summary.Dead, IncompleteRecords: summary.IncompleteRecords, CompletionRate: summary.CompletionRate, DeadRate: summary.DeadRate, AverageCompletionSeconds: summary.AverageCompletionSeconds, OldestQueuedSeconds: summary.OldestQueuedSeconds, Actions: queueDashboardActions(summary.Actions), ActivityCompletion: queueDashboardActivityCompletion(summary.ActivityCompletion), HourlyVolumes: queueDashboardHourlyVolumes(summary.HourlyVolumes), HourlyActionVolumes: queueDashboardHourlyActionVolumes(summary.HourlyActionVolumes), Operators: queueDashboardOperators(summary.Operators), FailureReasons: queueDashboardFailureReasons(summary.FailureReasons), RecentFailures: queueDashboardJobs(summary.RecentFailures), RecentJobs: queueDashboardJobs(summary.RecentJobs), LastUpdatedAt: optionalDashboardTime(summary.LastUpdatedAt)}
	return adminCapability{Status: "available", Detail: data.Scope, Data: data}
}

func (handler *AdminDashboardHandler) rabbitMQSnapshot(request *http.Request) adminCapability {
	if handler.rabbitMQ == nil {
		return adminCapability{Status: "not_configured", Detail: "RabbitMQ Management API is not configured."}
	}
	snapshot, err := handler.rabbitMQ.Snapshot(request.Context())
	if err != nil {
		handler.logger.Printf("Admin dashboard RabbitMQ read failed: %v", err)
		return adminCapability{Status: "source_unavailable", Detail: "RabbitMQ broker data is currently unavailable."}
	}
	return adminCapability{Status: "available", Detail: "Live RabbitMQ Management API snapshot.", Data: snapshot}
}

func unavailableCapability(detail string) adminCapability {
	return adminCapability{Status: "not_configured", Detail: detail}
}

func (handler *AdminDashboardHandler) tripFreshnessSummary(request *http.Request) adminCapability {
	if handler.tripFreshness == nil {
		return adminCapability{Status: "not_configured", Detail: "Future-trip freshness reporting is not configured."}
	}
	summary, err := handler.tripFreshness.Summary(request.Context())
	if err != nil {
		handler.logger.Printf("Admin dashboard future-trip freshness read failed: %v", err)
		return adminCapability{Status: "source_unavailable", Detail: "Future-trip freshness data is currently unavailable."}
	}
	return adminCapability{Status: "available", Detail: summary.Scope, Data: summary}
}
