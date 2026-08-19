package http

import (
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// NewRouter builds HTTP routing for the protected UI, ingestion, health, and persisted read APIs.
func NewRouter(tripDetailsService *master.TripDetailsService, orionmaxInventoryChangeService *master.OrionmaxInventoryEventService, readService *master.TripDetailsReadService, queueJobsService *master.QueueJobsService, tablesService *master.TablesService, uiAccessAuth *UIAccessAuth) http.Handler {
	readHandler := NewTripDetailsReadHandler(readService)
	queueJobsHandler := NewQueueJobsReportHandler(queueJobsService)
	tablesHandler := NewTablesHandler(tablesService)
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", redirectTo("/orbitplus/login"))
	mux.Handle("GET /orbitplus", redirectTo("/orbitplus/login"))
	mux.Handle("/orbitplus/login", http.HandlerFunc(uiAccessAuth.Login))
	mux.Handle("POST /orbitplus/logout", http.HandlerFunc(uiAccessAuth.Logout))
	mux.Handle("GET /orbitplus/{$}", uiAccessAuth.RequirePage(serveUIFile("ui/index.html")))
	mux.Handle("GET /orbitplus/tables", uiAccessAuth.RequirePage(serveUIFile("ui/tables/index.html")))
	mux.Handle("GET /orbitplus/tables/", redirectTo("/orbitplus/tables"))
	mux.Handle("GET /orbitplus/tables/periodic-refresh-routes", uiAccessAuth.RequirePage(serveUIFile("ui/tables/periodic-refresh-routes/index.html")))
	mux.Handle("GET /orbitplus/tables/route-metadata", uiAccessAuth.RequirePage(serveUIFile("ui/tables/route-metadata/index.html")))
	mux.Handle("GET /orbitplus/tables/schedule-metadata", uiAccessAuth.RequirePage(serveUIFile("ui/tables/schedule-metadata/index.html")))
	mux.Handle("GET /orbitplus/reports/queue-jobs", uiAccessAuth.RequirePage(serveUIFile("ui/queue-jobs/index.html")))
	mux.Handle("GET /orbitplus/reports/queue-jobs/", redirectTo("/orbitplus/reports/queue-jobs"))
	mux.Handle("GET /orbitplus/styles.css", uiAccessAuth.RequireEnabled(serveUIFile("ui/styles.css")))
	mux.Handle("GET /orbitplus/api/reports/queue-jobs", uiAccessAuth.RequireAPI(queueJobsHandler))
	mux.Handle("GET /orbitplus/api/tables/periodic-refresh-routes", uiAccessAuth.RequireAPI(http.HandlerFunc(tablesHandler.ServePeriodicRefreshRoutes)))
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
