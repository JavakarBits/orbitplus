package master

import (
	"context"
	"errors"
	"sort"
	"time"

	"orbitplusmaster/internal/domain"
)

const queueJobsReportLimit = 100

// QueueJobsService provides recent queue lifecycle records for the report UI.
type QueueJobsService struct {
	reader QueueMetrixReader
}

// QueueJobsSummary provides sample-scoped operational analytics for the admin portal.
type QueueJobsSummary struct {
	LoadedRecords            int
	Received                 int
	Queued                   int
	Completed                int
	Dead                     int
	IncompleteRecords        int
	CompletionRate           float64
	DeadRate                 float64
	AverageCompletionSeconds *int64
	OldestQueuedSeconds      *int64
	Actions                  []QueueJobsActionSummary
	ActivityCompletion       []QueueJobsActivityCompletionSummary
	HourlyVolumes            []QueueJobsHourlyVolumeSummary
	HourlyActionVolumes      []QueueJobsHourlyActionVolumeSummary
	Operators                []QueueJobsOperatorSummary
	FailureReasons           []QueueJobsFailureSummary
	RecentFailures           []domain.QueueMetrix
	RecentJobs               []domain.QueueMetrix
	LastUpdatedAt            time.Time
}

// QueueJobsActionSummary groups loaded lifecycle records by action type.
type QueueJobsActionSummary struct {
	ActionType string
	Count      int
}

// QueueJobsActivityCompletionSummary describes completed queue jobs grouped by activity type.
type QueueJobsActivityCompletionSummary struct {
	ActivityType             string
	CompletedJobs            int
	AverageCompletionSeconds int64
}

// QueueJobsHourlyVolumeSummary groups loaded lifecycle updates by UTC hour.
type QueueJobsHourlyVolumeSummary struct {
	Hour  time.Time
	Count int
}

// QueueJobsHourlyActionVolumeSummary groups hourly lifecycle updates by action type.
type QueueJobsHourlyActionVolumeSummary struct {
	Hour       time.Time
	ActionType string
	Count      int
}

// QueueJobsOperatorSummary groups loaded lifecycle records by operator.
type QueueJobsOperatorSummary struct {
	OperatorCode string
	Count        int
}

// QueueJobsFailureSummary groups failed records by their recorded reason.
type QueueJobsFailureSummary struct {
	Reason string
	Count  int
}

type queueJobsActivityCompletionAccumulator struct {
	completionTotal time.Duration
	completedJobs   int
}

// NewQueueJobsService constructs the queue report reader.
func NewQueueJobsService(reader QueueMetrixReader) *QueueJobsService {
	return &QueueJobsService{reader: reader}
}

// List returns a bounded Queue Metrics sample, sorted newest update first within that sample.
func (service *QueueJobsService) List(ctx context.Context) ([]domain.QueueMetrix, error) {
	if service == nil || service.reader == nil {
		return nil, errors.New("queue jobs reporting is not configured")
	}
	jobs, err := service.reader.List(ctx, queueJobsReportLimit)
	if err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(left, right int) bool { return jobs[left].UpdatedAt.After(jobs[right].UpdatedAt) })
	return jobs, nil
}

// Summary derives last-24-hour analytics from the loaded Queue Metrics sample.
func (service *QueueJobsService) Summary(ctx context.Context) (QueueJobsSummary, error) {
	now := time.Now().UTC()
	jobs, err := service.List(ctx)
	if err != nil {
		return QueueJobsSummary{}, err
	}
	jobs = queueJobsUpdatedSince(jobs, now.Add(-24*time.Hour))
	summary := QueueJobsSummary{
		LoadedRecords: len(jobs), Actions: make([]QueueJobsActionSummary, 0), ActivityCompletion: make([]QueueJobsActivityCompletionSummary, 0),
		HourlyVolumes: queueJobsHourlyVolumes(now, jobs), HourlyActionVolumes: queueJobsHourlyActionVolumes(now, jobs), Operators: make([]QueueJobsOperatorSummary, 0), FailureReasons: make([]QueueJobsFailureSummary, 0),
		RecentFailures: make([]domain.QueueMetrix, 0, 5), RecentJobs: make([]domain.QueueMetrix, 0, 5),
	}
	actions := make(map[string]int)
	activityCompletion := make(map[string]queueJobsActivityCompletionAccumulator)
	operators := make(map[string]int)
	failureReasons := make(map[string]int)
	var completionTotal time.Duration
	var completionCount int64
	for _, job := range jobs {
		if len(summary.RecentJobs) < cap(summary.RecentJobs) {
			summary.RecentJobs = append(summary.RecentJobs, job)
		}
		if job.UpdatedAt.After(summary.LastUpdatedAt) {
			summary.LastUpdatedAt = job.UpdatedAt
		}
		switch job.QueueStatus {
		case domain.QueueStatusReceived:
			summary.Received++
		case domain.QueueStatusQueued:
			summary.Queued++
		case domain.QueueStatusCompleted:
			summary.Completed++
		case domain.QueueStatusDead:
			summary.Dead++
		}
		if queueJobHasMissingOperationalFields(job) {
			summary.IncompleteRecords++
		}
		actionType := job.ActionType
		if actionType == "" {
			actionType = "Unknown action"
		}
		actions[actionType]++
		operatorCode := job.OperatorCode
		if operatorCode == "" {
			operatorCode = "Unknown operator"
		}
		operators[operatorCode]++
		if !job.QueuedAt.IsZero() && !job.CompletedAt.IsZero() && !job.CompletedAt.Before(job.QueuedAt) {
			elapsed := job.CompletedAt.Sub(job.QueuedAt)
			completionTotal += elapsed
			completionCount++
			activityType := job.ActivityType
			if activityType == "" {
				activityType = "Unknown activity"
			}
			current := activityCompletion[activityType]
			current.completionTotal += elapsed
			current.completedJobs++
			activityCompletion[activityType] = current
		}
		if job.QueueStatus == domain.QueueStatusQueued && !job.QueuedAt.IsZero() {
			age := int64(now.Sub(job.QueuedAt).Seconds())
			if age > 0 && (summary.OldestQueuedSeconds == nil || age > *summary.OldestQueuedSeconds) {
				ageCopy := age
				summary.OldestQueuedSeconds = &ageCopy
			}
		}
		if job.QueueStatus == domain.QueueStatusDead || job.FailureMessage != "" {
			reason := job.FailureMessage
			if reason == "" {
				reason = "No failure message recorded"
			}
			failureReasons[reason]++
			if len(summary.RecentFailures) < cap(summary.RecentFailures) {
				summary.RecentFailures = append(summary.RecentFailures, job)
			}
		}
	}
	if summary.LoadedRecords > 0 {
		summary.CompletionRate = float64(summary.Completed) / float64(summary.LoadedRecords) * 100
		summary.DeadRate = float64(summary.Dead) / float64(summary.LoadedRecords) * 100
	}
	if completionCount > 0 {
		seconds := int64(completionTotal.Seconds()) / completionCount
		summary.AverageCompletionSeconds = &seconds
	}
	for actionType, count := range actions {
		summary.Actions = append(summary.Actions, QueueJobsActionSummary{ActionType: actionType, Count: count})
	}
	for activityType, totals := range activityCompletion {
		summary.ActivityCompletion = append(summary.ActivityCompletion, QueueJobsActivityCompletionSummary{
			ActivityType: activityType, CompletedJobs: totals.completedJobs,
			AverageCompletionSeconds: int64(totals.completionTotal.Seconds()) / int64(totals.completedJobs),
		})
	}
	for operatorCode, count := range operators {
		summary.Operators = append(summary.Operators, QueueJobsOperatorSummary{OperatorCode: operatorCode, Count: count})
	}
	for reason, count := range failureReasons {
		summary.FailureReasons = append(summary.FailureReasons, QueueJobsFailureSummary{Reason: reason, Count: count})
	}
	sort.Slice(summary.Actions, func(left, right int) bool { return summary.Actions[left].Count > summary.Actions[right].Count })
	sort.Slice(summary.ActivityCompletion, func(left, right int) bool {
		return summary.ActivityCompletion[left].ActivityType < summary.ActivityCompletion[right].ActivityType
	})
	sort.Slice(summary.Operators, func(left, right int) bool { return summary.Operators[left].Count > summary.Operators[right].Count })
	sort.Slice(summary.FailureReasons, func(left, right int) bool {
		return summary.FailureReasons[left].Count > summary.FailureReasons[right].Count
	})
	return summary, nil
}

func queueJobsHourlyVolumes(now time.Time, jobs []domain.QueueMetrix) []QueueJobsHourlyVolumeSummary {
	currentHour := now.UTC().Truncate(time.Hour)
	firstHour := currentHour.Add(-23 * time.Hour)
	counts := make(map[time.Time]int, 24)
	for _, job := range jobs {
		hour := job.UpdatedAt.UTC().Truncate(time.Hour)
		if !hour.Before(firstHour) && !hour.After(currentHour) {
			counts[hour]++
		}
	}
	volumes := make([]QueueJobsHourlyVolumeSummary, 0, 24)
	for hour := firstHour; !hour.After(currentHour); hour = hour.Add(time.Hour) {
		volumes = append(volumes, QueueJobsHourlyVolumeSummary{Hour: hour, Count: counts[hour]})
	}
	return volumes
}

func queueJobsUpdatedSince(jobs []domain.QueueMetrix, since time.Time) []domain.QueueMetrix {
	filtered := make([]domain.QueueMetrix, 0, len(jobs))
	for _, job := range jobs {
		if !job.UpdatedAt.IsZero() && !job.UpdatedAt.Before(since) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func queueJobHasMissingOperationalFields(job domain.QueueMetrix) bool {
	return job.ActivityType == "" || job.ActionType == "" || job.OperatorCode == "" || job.SourceStationCode == "" || job.DestinationStationCode == "" || job.TripDate == ""
}

func queueJobsHourlyActionVolumes(now time.Time, jobs []domain.QueueMetrix) []QueueJobsHourlyActionVolumeSummary {
	currentHour := now.UTC().Truncate(time.Hour)
	firstHour := currentHour.Add(-23 * time.Hour)
	counts := make(map[time.Time]map[string]int)
	for _, job := range jobs {
		hour := job.UpdatedAt.UTC().Truncate(time.Hour)
		if hour.Before(firstHour) || hour.After(currentHour) {
			continue
		}
		actionType := job.ActionType
		if actionType == "" {
			actionType = "Unknown action"
		}
		if counts[hour] == nil {
			counts[hour] = make(map[string]int)
		}
		counts[hour][actionType]++
	}
	result := make([]QueueJobsHourlyActionVolumeSummary, 0)
	for hour := firstHour; !hour.After(currentHour); hour = hour.Add(time.Hour) {
		for actionType, count := range counts[hour] {
			result = append(result, QueueJobsHourlyActionVolumeSummary{Hour: hour, ActionType: actionType, Count: count})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Hour.Equal(result[right].Hour) {
			return result[left].ActionType < result[right].ActionType
		}
		return result[left].Hour.Before(result[right].Hour)
	})
	return result
}
