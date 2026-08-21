package http

import (
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

// QueueDashboardHandler returns sample-scoped Queue Metrics analytics for the protected admin portal.
type QueueDashboardHandler struct {
	service *master.QueueJobsService
	logger  *log.Logger
}

// NewQueueDashboardHandler constructs the queue dashboard API handler.
func NewQueueDashboardHandler(service *master.QueueJobsService) *QueueDashboardHandler {
	return &QueueDashboardHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns last-24-hour analytics from the loaded Queue Metrics sample.
func (handler *QueueDashboardHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Queue analytics is not configured")
		return
	}
	summary, err := handler.service.Summary(request.Context())
	if err != nil {
		handler.logger.Printf("Queue dashboard read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load queue analytics")
		return
	}
	writeJSONData(response, queueDashboardResponse{
		Scope:                    "Queue Metrics updates from the last 24 hours within the loaded sample (maximum 100)",
		LoadedRecords:            summary.LoadedRecords,
		Received:                 summary.Received,
		Queued:                   summary.Queued,
		Completed:                summary.Completed,
		Dead:                     summary.Dead,
		IncompleteRecords:        summary.IncompleteRecords,
		CompletionRate:           summary.CompletionRate,
		DeadRate:                 summary.DeadRate,
		AverageCompletionSeconds: summary.AverageCompletionSeconds,
		OldestQueuedSeconds:      summary.OldestQueuedSeconds,
		Actions:                  queueDashboardActions(summary.Actions),
		ActivityCompletion:       queueDashboardActivityCompletion(summary.ActivityCompletion),
		HourlyVolumes:            queueDashboardHourlyVolumes(summary.HourlyVolumes),
		HourlyActionVolumes:      queueDashboardHourlyActionVolumes(summary.HourlyActionVolumes),
		Operators:                queueDashboardOperators(summary.Operators),
		FailureReasons:           queueDashboardFailureReasons(summary.FailureReasons),
		RecentFailures:           queueDashboardJobs(summary.RecentFailures),
		RecentJobs:               queueDashboardJobs(summary.RecentJobs),
		LastUpdatedAt:            optionalDashboardTime(summary.LastUpdatedAt),
	})
}

type queueDashboardResponse struct {
	Scope                    string                                 `json:"scope"`
	LoadedRecords            int                                    `json:"loadedRecords"`
	Received                 int                                    `json:"received"`
	Queued                   int                                    `json:"queued"`
	Completed                int                                    `json:"completed"`
	Dead                     int                                    `json:"dead"`
	IncompleteRecords        int                                    `json:"incompleteRecords"`
	CompletionRate           float64                                `json:"completionRate"`
	DeadRate                 float64                                `json:"deadRate"`
	AverageCompletionSeconds *int64                                 `json:"averageCompletionSeconds"`
	OldestQueuedSeconds      *int64                                 `json:"oldestQueuedSeconds"`
	Actions                  []queueDashboardActionItem             `json:"actions"`
	ActivityCompletion       []queueDashboardActivityCompletionItem `json:"activityCompletion"`
	HourlyVolumes            []queueDashboardHourlyVolumeItem       `json:"hourlyVolumes"`
	HourlyActionVolumes      []queueDashboardHourlyActionVolumeItem `json:"hourlyActionVolumes"`
	Operators                []queueDashboardOperatorItem           `json:"operators"`
	FailureReasons           []queueDashboardFailureItem            `json:"failureReasons"`
	RecentFailures           []queueJobReportItem                   `json:"recentFailures"`
	RecentJobs               []queueJobReportItem                   `json:"recentJobs"`
	LastUpdatedAt            *time.Time                             `json:"lastUpdatedAt"`
}

type queueDashboardActionItem struct {
	ActionType string `json:"actionType"`
	Count      int    `json:"count"`
}

type queueDashboardActivityCompletionItem struct {
	ActivityType             string `json:"activityType"`
	CompletedJobs            int    `json:"completedJobs"`
	AverageCompletionSeconds int64  `json:"averageCompletionSeconds"`
}

type queueDashboardHourlyVolumeItem struct {
	Hour  time.Time `json:"hour"`
	Count int       `json:"count"`
}

type queueDashboardHourlyActionVolumeItem struct {
	Hour       time.Time `json:"hour"`
	ActionType string    `json:"actionType"`
	Count      int       `json:"count"`
}

type queueDashboardOperatorItem struct {
	OperatorCode string `json:"operatorCode"`
	Count        int    `json:"count"`
}

type queueDashboardFailureItem struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func queueDashboardActions(items []master.QueueJobsActionSummary) []queueDashboardActionItem {
	result := make([]queueDashboardActionItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardActionItem{ActionType: item.ActionType, Count: item.Count})
	}
	return result
}

func queueDashboardActivityCompletion(items []master.QueueJobsActivityCompletionSummary) []queueDashboardActivityCompletionItem {
	result := make([]queueDashboardActivityCompletionItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardActivityCompletionItem{ActivityType: item.ActivityType, CompletedJobs: item.CompletedJobs, AverageCompletionSeconds: item.AverageCompletionSeconds})
	}
	return result
}

func queueDashboardHourlyVolumes(items []master.QueueJobsHourlyVolumeSummary) []queueDashboardHourlyVolumeItem {
	result := make([]queueDashboardHourlyVolumeItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardHourlyVolumeItem{Hour: item.Hour, Count: item.Count})
	}
	return result
}

func queueDashboardHourlyActionVolumes(items []master.QueueJobsHourlyActionVolumeSummary) []queueDashboardHourlyActionVolumeItem {
	result := make([]queueDashboardHourlyActionVolumeItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardHourlyActionVolumeItem{Hour: item.Hour, ActionType: item.ActionType, Count: item.Count})
	}
	return result
}

func queueDashboardOperators(items []master.QueueJobsOperatorSummary) []queueDashboardOperatorItem {
	result := make([]queueDashboardOperatorItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardOperatorItem{OperatorCode: item.OperatorCode, Count: item.Count})
	}
	return result
}

func queueDashboardFailureReasons(items []master.QueueJobsFailureSummary) []queueDashboardFailureItem {
	result := make([]queueDashboardFailureItem, 0, len(items))
	for _, item := range items {
		result = append(result, queueDashboardFailureItem{Reason: item.Reason, Count: item.Count})
	}
	return result
}

func queueDashboardJobs(items []domain.QueueMetrix) []queueJobReportItem {
	result := make([]queueJobReportItem, 0, len(items))
	for _, item := range items {
		result = append(result, newQueueJobReportItem(item))
	}
	return result
}

func optionalDashboardTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
