package http

import (
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/rabbitmq"
)

// NewRouter builds HTTP routing for the protected UI, ingestion, health, and persisted read APIs.
func NewRouter(startedAt time.Time, tripDetailsService *master.TripDetailsService, orionmaxInventoryChangeService *master.OrionmaxInventoryEventService, readService *master.TripDetailsReadService, cacheService *master.CacheReadService, rabbitMQManagement rabbitmq.ManagementReader, queueJobsService *master.QueueJobsService, tripFreshnessService *master.TripFreshnessService, tripHistoryService *master.TripHistoryService, tablesService *master.TablesService, operatorRegistry master.OperatorRegistry, uiAccessAuth *UIAccessAuth) http.Handler {
	readHandler := NewTripDetailsReadHandler(readService)
	queueJobsHandler := NewQueueJobsReportHandler(queueJobsService)
	queueDashboardHandler := NewQueueDashboardHandler(queueJobsService)
	systemHealthHandler := NewSystemHealthHandler(startedAt)
	adminDashboardHandler := NewAdminDashboardHandler(systemHealthHandler, queueJobsService, tripFreshnessService, rabbitMQManagement)
	cacheHandler := NewCacheHandler(cacheService)
	rabbitMQDashboardHandler := NewRabbitMQDashboardHandler(rabbitMQManagement)
	tablesHandler := NewTablesHandler(tablesService)
	tripHistoryHandler := NewTripHistoryHandler(tripHistoryService)
	operatorsHandler := NewOperatorsHandler(operatorRegistry)
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", redirectTo("/orbitplus/login"))
	mux.Handle("GET /orbitplus", redirectTo("/orbitplus/login"))
	mux.Handle("/orbitplus/login", http.HandlerFunc(uiAccessAuth.Login))
	mux.Handle("POST /orbitplus/logout", http.HandlerFunc(uiAccessAuth.Logout))
	mux.Handle("GET /orbitplus/{$}", redirectTo("/orbitplus/admin"))
	mux.Handle("GET /orbitplus/admin", uiAccessAuth.RequirePage(serveUIFile("ui/admin/index.html")))
	mux.Handle("GET /orbitplus/admin/", redirectTo("/orbitplus/admin"))
	mux.Handle("GET /orbitplus/admin/{page}", uiAccessAuth.RequirePage(serveUIFile("ui/admin/index.html")))
	mux.Handle("GET /orbitplus/admin/portal.css", uiAccessAuth.RequireEnabled(serveUIFile("ui/admin/portal.css")))
	mux.Handle("GET /orbitplus/admin/portal-service.js", uiAccessAuth.RequireEnabled(serveUIFile("ui/admin/portal-service.js")))
	mux.Handle("GET /orbitplus/admin/portal.js", uiAccessAuth.RequireEnabled(serveUIFile("ui/admin/portal.js")))
	mux.Handle("GET /orbitplus/master", redirectTo("/orbitplus/admin"))
	mux.Handle("GET /orbitplus/reports", uiAccessAuth.RequirePage(serveUIFile("ui/reports/index.html")))
	mux.Handle("GET /orbitplus/reports/queue-analytics", uiAccessAuth.RequirePage(serveUIFile("ui/reports/queue-analytics/index.html")))
	mux.Handle("GET /orbitplus/tables", uiAccessAuth.RequirePage(serveUIFile("ui/tables/index.html")))
	mux.Handle("GET /orbitplus/tables/", redirectTo("/orbitplus/tables"))
	mux.Handle("GET /orbitplus/tables/route-metadata", uiAccessAuth.RequirePage(serveUIFile("ui/tables/route-metadata/index.html")))
	mux.Handle("GET /orbitplus/tables/schedule-metadata", uiAccessAuth.RequirePage(serveUIFile("ui/tables/schedule-metadata/index.html")))
	mux.Handle("GET /orbitplus/reports/queue-jobs", uiAccessAuth.RequirePage(serveUIFile("ui/queue-jobs/index.html")))
	mux.Handle("GET /orbitplus/reports/queue-jobs/", redirectTo("/orbitplus/reports/queue-jobs"))
	mux.Handle("GET /orbitplus/cache", uiAccessAuth.RequirePage(serveUIFile("ui/cache/index.html")))
	mux.Handle("GET /orbitplus/cache/", redirectTo("/orbitplus/cache"))
	mux.Handle("GET /orbitplus/rabbitmq", uiAccessAuth.RequirePage(serveUIFile("ui/rabbitmq/index.html")))
	mux.Handle("GET /orbitplus/rabbitmq/", redirectTo("/orbitplus/rabbitmq"))
	mux.Handle("GET /orbitplus/styles.css", uiAccessAuth.RequireEnabled(serveUIFile("ui/styles.css")))
	mux.Handle("GET /orbitplus/api/reports/queue-jobs", uiAccessAuth.RequireAPI(queueJobsHandler))
	mux.Handle("GET /orbitplus/api/dashboard/queue-summary", uiAccessAuth.RequireAPI(queueDashboardHandler))
	mux.Handle("GET /orbitplus/api/dashboard/system-health", uiAccessAuth.RequireAPI(systemHealthHandler))
	mux.Handle("GET /orbitplus/api/admin/dashboard", uiAccessAuth.RequireAPI(adminDashboardHandler))
	mux.Handle("GET /orbitplus/api/admin/trip-history", uiAccessAuth.RequireAPI(tripHistoryHandler))
	mux.Handle("GET /orbitplus/api/admin/operators", uiAccessAuth.RequireAPI(http.HandlerFunc(operatorsHandler.ServeList)))
	mux.Handle("POST /orbitplus/api/admin/operators", uiAccessAuth.RequireAPI(http.HandlerFunc(operatorsHandler.ServeCreate)))
	mux.Handle("PATCH /orbitplus/api/admin/operators/{operatorCode}", uiAccessAuth.RequireAPI(http.HandlerFunc(operatorsHandler.ServeStatus)))
	mux.Handle("GET /orbitplus/api/cache", uiAccessAuth.RequireAPI(cacheHandler))
	mux.Handle("GET /orbitplus/api/cache/value", uiAccessAuth.RequireAPI(http.HandlerFunc(cacheHandler.ServeValue)))
	mux.Handle("GET /orbitplus/api/rabbitmq", uiAccessAuth.RequireAPI(rabbitMQDashboardHandler))
	mux.Handle("GET /orbitplus/api/tables/route-metadata", uiAccessAuth.RequireAPI(http.HandlerFunc(tablesHandler.ServeRouteMetadata)))
	mux.Handle("GET /orbitplus/api/tables/schedule-metadata", uiAccessAuth.RequireAPI(http.HandlerFunc(tablesHandler.ServeScheduleMetadata)))

	mux.Handle("POST /api/tripdetails", NewTripDetailsHandler(tripDetailsService))
	mux.Handle("POST /orbitplus/api/tripdetails/dlq", NewTripDetailsDLQHandler(tripDetailsService))
	mux.Handle("POST /api/orionmax/inventory/events", NewOrionmaxInventoryChangeHandler(orionmaxInventoryChangeService))
	mux.Handle("GET /health", NewHealthHandler())
	mux.Handle("GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}", http.HandlerFunc(readHandler.ServeSearch))
	mux.Handle("GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}", http.HandlerFunc(readHandler.ServeBusMap))
	mux.Handle("GET /orbitplus/api/3.0/json/", http.HandlerFunc(readHandler.ServeInvalidRoute))
	return mux
}
