package http

import (
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

// QueueJobsReportHandler returns queue lifecycle records for the report UI.
type QueueJobsReportHandler struct {
	service *master.QueueJobsService
	logger  *log.Logger
}

// NewQueueJobsReportHandler constructs the report API handler.
func NewQueueJobsReportHandler(service *master.QueueJobsService) *QueueJobsReportHandler {
	return &QueueJobsReportHandler{service: service, logger: log.Default()}
}

// ServeHTTP lists the bounded set of queue job report records.
func (handler *QueueJobsReportHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Queue jobs reporting is not configured")
		return
	}
	jobs, err := handler.service.List(request.Context())
	if err != nil {
		handler.logger.Printf("Queue jobs report read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load queue jobs")
		return
	}
	items := make([]queueJobReportItem, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, newQueueJobReportItem(job))
	}
	writeJSONData(response, items)
}

type queueJobReportItem struct {
	ReferenceID    string     `json:"referenceId"`
	ActivityType   string     `json:"activityType"`
	ActionType     string     `json:"actionType"`
	OperatorCode   string     `json:"operatorCode"`
	FromStation    string     `json:"fromStation"`
	ToStation      string     `json:"toStation"`
	TripDate       string     `json:"tripDate"`
	QueueStatus    string     `json:"queueStatus"`
	QueuedAt       *time.Time `json:"queuedAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	DeadLetteredAt *time.Time `json:"deadLetteredAt"`
	UpdatedAt      *time.Time `json:"updatedAt"`
	FailureMessage string     `json:"failureMessage"`
}

func newQueueJobReportItem(job domain.QueueMetrix) queueJobReportItem {
	return queueJobReportItem{
		ReferenceID: job.ReferenceID, ActivityType: job.ActivityType, ActionType: job.ActionType,
		OperatorCode: job.OperatorCode, FromStation: job.SourceStationCode, ToStation: job.DestinationStationCode,
		TripDate: job.TripDate, QueueStatus: job.QueueStatus, QueuedAt: optionalReportTime(job.QueuedAt),
		CompletedAt: optionalReportTime(job.CompletedAt), DeadLetteredAt: optionalReportTime(job.DeadLetteredAt),
		UpdatedAt: optionalReportTime(job.UpdatedAt), FailureMessage: job.FailureMessage,
	}
}

func optionalReportTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
