package master

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"orbitplusmaster/internal/domain"
)

// Guardrails for the diagnostic queue_metrix scan. queue_metrix is keyed by
// reference_id only, so a trip or route lookup must walk the table. These caps
// keep one lookup bounded in rows, time, and concurrency.
const (
	tripHistoryPageSize        = 5000
	tripHistoryMaxRowsExamined = 200000
	tripHistoryMaxMatches      = 500
	tripHistoryTimeout         = 10 * time.Second
)

var (
	// ErrInvalidTripHistoryLookup indicates a missing or unsafe trip history key.
	ErrInvalidTripHistoryLookup = errors.New("invalid trip history lookup")
	// ErrTripHistoryNotConfigured indicates queue lifecycle storage is unavailable.
	ErrTripHistoryNotConfigured = errors.New("trip history reporting is not configured")
	// ErrTripHistoryBusy indicates another trip scan is already running.
	ErrTripHistoryBusy = errors.New("another trip history scan is in progress")
)

// TripHistoryQuery selects queue lifecycle records for one trip or route.
// Only busmap-family records carry a trip code, so the route fields exist to
// reach search-family records, which are stored against operator, date, and stations.
type TripHistoryQuery struct {
	OperatorCode string
	TripCode     string
	TripDate     string
	FromStation  string
	ToStation    string
}

// TripHistoryScanOptions bounds one diagnostic queue_metrix scan.
type TripHistoryScanOptions struct {
	PageSize        int
	MaxRowsExamined int
	MaxMatches      int
}

// TripHistoryScanResult carries matched records and the cost of the scan.
type TripHistoryScanResult struct {
	Records      []domain.QueueMetrix
	RowsExamined int
	Truncated    bool
}

// TripHistoryQueueReader scans queue lifecycle records for one trip or route.
type TripHistoryQueueReader interface {
	ScanTripHistory(ctx context.Context, query TripHistoryQuery, options TripHistoryScanOptions) (TripHistoryScanResult, error)
}

// TripHistoryService returns queue lifecycle history using a bounded scan.
// Only one scan runs at a time so the lookup cannot be amplified.
type TripHistoryService struct {
	reader  TripHistoryQueueReader
	running chan struct{}
	now     func() time.Time
}

// HasRouteFilters reports whether any route-scoped selector was supplied.
func (query TripHistoryQuery) HasRouteFilters() bool {
	return query.TripDate != "" || query.FromStation != "" || query.ToStation != ""
}

// Matches reports whether one record belongs to the requested trip or route.
// A trip code match and a route match are combined as a union so every activity
// type recorded for the trip is visible, including records without a trip code.
func (query TripHistoryQuery) Matches(record domain.QueueMetrix) bool {
	if !strings.EqualFold(record.OperatorCode, query.OperatorCode) {
		return false
	}
	if query.TripCode != "" && strings.EqualFold(record.TripCode, query.TripCode) {
		return true
	}
	return query.HasRouteFilters() && query.matchesRoute(record)
}

func (query TripHistoryQuery) matchesRoute(record domain.QueueMetrix) bool {
	if query.TripDate != "" && !strings.EqualFold(record.TripDate, query.TripDate) {
		return false
	}
	if query.FromStation != "" && !strings.EqualFold(record.SourceStationCode, query.FromStation) {
		return false
	}
	if query.ToStation != "" && !strings.EqualFold(record.DestinationStationCode, query.ToStation) {
		return false
	}
	return true
}

func (query TripHistoryQuery) normalized() TripHistoryQuery {
	return TripHistoryQuery{
		OperatorCode: strings.TrimSpace(query.OperatorCode), TripCode: strings.TrimSpace(query.TripCode),
		TripDate: strings.TrimSpace(query.TripDate), FromStation: strings.TrimSpace(query.FromStation),
		ToStation: strings.TrimSpace(query.ToStation),
	}
}

func (query TripHistoryQuery) valid() bool {
	for _, value := range []string{query.OperatorCode, query.TripCode, query.TripDate, query.FromStation, query.ToStation} {
		if value != "" && !validTripHistoryKey(value) {
			return false
		}
	}
	return query.OperatorCode != "" && (query.TripCode != "" || query.HasRouteFilters())
}

// NewTripHistoryService constructs the bounded trip history lookup service.
func NewTripHistoryService(reader TripHistoryQueueReader) *TripHistoryService {
	return &TripHistoryService{reader: reader, running: make(chan struct{}, 1), now: time.Now}
}

// TripHistoryEntry describes one queue_metrix record belonging to the trip or route.
type TripHistoryEntry struct {
	ReferenceID      string     `json:"referenceId"`
	ActivityType     string     `json:"activityType"`
	ActionType       string     `json:"actionType"`
	ScheduleCode     string     `json:"scheduleCode"`
	TripCode         string     `json:"tripCode"`
	UpdatedTripCodes []string   `json:"updatedTripCodes"`
	Message          string     `json:"message"`
	FromStation      string     `json:"fromStation"`
	ToStation        string     `json:"toStation"`
	TripDate         string     `json:"tripDate"`
	Zone             string     `json:"zone"`
	QueueStatus      string     `json:"queueStatus"`
	FailureMessage   string     `json:"failureMessage"`
	QueuedAt         *time.Time `json:"queuedAt"`
	CompletedAt      *time.Time `json:"completedAt"`
	DeadLetteredAt   *time.Time `json:"deadLetteredAt"`
	UpdatedAt        *time.Time `json:"updatedAt"`
	DurationSeconds  *int64     `json:"durationSeconds"`
}

// TripHistoryActivitySummary counts matched records for one activity type.
type TripHistoryActivitySummary struct {
	ActivityType string `json:"activityType"`
	Count        int    `json:"count"`
}

// TripHistorySummary aggregates the queue lifecycle history of one trip or route.
type TripHistorySummary struct {
	OperatorCode           string                       `json:"operatorCode"`
	TripCode               string                       `json:"tripCode"`
	TripDate               string                       `json:"tripDate"`
	FromStation            string                       `json:"fromStation"`
	ToStation              string                       `json:"toStation"`
	TotalRecords           int                          `json:"totalRecords"`
	Completed              int                          `json:"completed"`
	Dead                   int                          `json:"dead"`
	Pending                int                          `json:"pending"`
	RowsExamined           int                          `json:"rowsExamined"`
	Truncated              bool                         `json:"truncated"`
	ElapsedMilliseconds    int64                        `json:"elapsedMilliseconds"`
	AverageDurationSeconds *int64                       `json:"averageDurationSeconds"`
	LongestDurationSeconds *int64                       `json:"longestDurationSeconds"`
	FirstQueuedAt          *time.Time                   `json:"firstQueuedAt"`
	LastActivityAt         *time.Time                   `json:"lastActivityAt"`
	Activities             []TripHistoryActivitySummary `json:"activities"`
	Entries                []TripHistoryEntry           `json:"entries"`
	Scope                  string                       `json:"scope"`
}

// Lookup returns queue lifecycle records for one trip or route, oldest first.
func (service *TripHistoryService) Lookup(ctx context.Context, query TripHistoryQuery) (TripHistorySummary, error) {
	if service == nil || service.reader == nil {
		return TripHistorySummary{}, ErrTripHistoryNotConfigured
	}
	query = query.normalized()
	if !query.valid() {
		return TripHistorySummary{}, ErrInvalidTripHistoryLookup
	}
	select {
	case service.running <- struct{}{}:
		defer func() { <-service.running }()
	default:
		return TripHistorySummary{}, ErrTripHistoryBusy
	}
	scanCtx, cancel := context.WithTimeout(ctx, tripHistoryTimeout)
	defer cancel()
	startedAt := service.now()
	scan, err := service.reader.ScanTripHistory(scanCtx, query, TripHistoryScanOptions{
		PageSize: tripHistoryPageSize, MaxRowsExamined: tripHistoryMaxRowsExamined, MaxMatches: tripHistoryMaxMatches,
	})
	if err != nil {
		return TripHistorySummary{}, err
	}
	records := scan.Records
	sort.Slice(records, func(left, right int) bool {
		return tripHistoryOrderTime(records[left]).Before(tripHistoryOrderTime(records[right]))
	})
	summary := TripHistorySummary{
		OperatorCode: query.OperatorCode, TripCode: query.TripCode, TripDate: query.TripDate,
		FromStation: query.FromStation, ToStation: query.ToStation, TotalRecords: len(records),
		RowsExamined: scan.RowsExamined, Truncated: scan.Truncated,
		ElapsedMilliseconds: service.now().Sub(startedAt).Milliseconds(),
		Activities:          make([]TripHistoryActivitySummary, 0),
		Entries:             make([]TripHistoryEntry, 0, len(records)),
		Scope:               tripHistoryScope(scan),
	}
	activities := make(map[string]int)
	var durationTotal int64
	var durationCount int64
	for _, record := range records {
		entry := newTripHistoryEntry(record)
		if entry.DurationSeconds != nil {
			durationTotal += *entry.DurationSeconds
			durationCount++
			if summary.LongestDurationSeconds == nil || *entry.DurationSeconds > *summary.LongestDurationSeconds {
				longest := *entry.DurationSeconds
				summary.LongestDurationSeconds = &longest
			}
		}
		switch record.QueueStatus {
		case domain.QueueStatusCompleted:
			summary.Completed++
		case domain.QueueStatusDead:
			summary.Dead++
		default:
			summary.Pending++
		}
		activityType := record.ActivityType
		if activityType == "" {
			activityType = "Unknown activity"
		}
		activities[activityType]++
		if !record.QueuedAt.IsZero() && (summary.FirstQueuedAt == nil || record.QueuedAt.Before(*summary.FirstQueuedAt)) {
			first := record.QueuedAt
			summary.FirstQueuedAt = &first
		}
		if latest := tripHistoryOrderTime(record); !latest.IsZero() && (summary.LastActivityAt == nil || latest.After(*summary.LastActivityAt)) {
			last := latest
			summary.LastActivityAt = &last
		}
		summary.Entries = append(summary.Entries, entry)
	}
	if durationCount > 0 {
		average := durationTotal / durationCount
		summary.AverageDurationSeconds = &average
	}
	for activityType, count := range activities {
		summary.Activities = append(summary.Activities, TripHistoryActivitySummary{ActivityType: activityType, Count: count})
	}
	sort.Slice(summary.Activities, func(left, right int) bool {
		if summary.Activities[left].Count == summary.Activities[right].Count {
			return summary.Activities[left].ActivityType < summary.Activities[right].ActivityType
		}
		return summary.Activities[left].Count > summary.Activities[right].Count
	})
	return summary, nil
}

func newTripHistoryEntry(record domain.QueueMetrix) TripHistoryEntry {
	updatedTripCodes := append([]string(nil), record.UpdatedTripCodes...)
	sort.Strings(updatedTripCodes)
	entry := TripHistoryEntry{
		ReferenceID: record.ReferenceID, ActivityType: record.ActivityType, ActionType: record.ActionType,
		ScheduleCode: record.ScheduleCode, TripCode: record.TripCode, UpdatedTripCodes: updatedTripCodes,
		Message: string(record.WorkerPayload), FromStation: record.SourceStationCode, ToStation: record.DestinationStationCode, TripDate: record.TripDate, Zone: record.Zone,
		QueueStatus: record.QueueStatus, FailureMessage: record.FailureMessage,
		QueuedAt: optionalTripHistoryTime(record.QueuedAt), CompletedAt: optionalTripHistoryTime(record.CompletedAt),
		DeadLetteredAt: optionalTripHistoryTime(record.DeadLetteredAt), UpdatedAt: optionalTripHistoryTime(record.UpdatedAt),
	}
	finished := record.CompletedAt
	if finished.IsZero() {
		finished = record.DeadLetteredAt
	}
	if !record.QueuedAt.IsZero() && !finished.IsZero() && !finished.Before(record.QueuedAt) {
		seconds := int64(finished.Sub(record.QueuedAt).Seconds())
		entry.DurationSeconds = &seconds
	}
	return entry
}

func tripHistoryScope(scan TripHistoryScanResult) string {
	if scan.Truncated {
		return "Partial result. queue_metrix is keyed by reference ID only, so this lookup scans the table and stopped at its row, match, or time limit. Narrow the search for a complete answer."
	}
	return "Complete match set for the supplied selectors. Only busmap-family records store a trip code, so add trip date and stations to include search-family activities."
}

func tripHistoryOrderTime(record domain.QueueMetrix) time.Time {
	if !record.QueuedAt.IsZero() {
		return record.QueuedAt
	}
	return record.UpdatedAt
}

func optionalTripHistoryTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func validTripHistoryKey(value string) bool {
	if len(value) > 128 {
		return false
	}
	return !strings.ContainsAny(value, " \t\r\n'\"")
}
