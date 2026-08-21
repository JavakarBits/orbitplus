package master

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"orbitplusmaster/internal/domain"
	"strings"
	"time"
)

var (
	ErrInventoryActivityTypeMismatch = errors.New("inventory activity types do not match")
	ErrUnsupportedInventoryActivity  = errors.New("unsupported inventory activity")
	ErrInvalidInventoryEvent         = errors.New("invalid inventory event")
	ErrTripCodeUnavailable           = errors.New("trip code unavailable for schedule")
)

var actionTypeByActivityType = map[string]string{
	"daily-job": "searchbusmap", "ticket-tentative-block-cancel": "busmap", "block-release": "busmap",
	"quick-fare-change": "search", "dp-fare-change": "search", "seat-block-release": "busmap",
	"ticket-cancel": "busmap", "manual-refresh": "searchbusmap", "notify-schedule": "searchbusmap",
	"fare-change": "search", "phone-ticket-cancel": "busmap", "not-travel": "search",
	"service_inactive": "search", "vsd_toggle": "busmap", "no_show": "busmap", "shift": "searchbusmap",
	"push_trip_info": "searchbusmap",
}

// InventoryScheduleReader resolves Bits trip codes from stored schedule metadata.
type InventoryScheduleReader interface {
	FindStagesBySchedule(ctx context.Context, operatorCode, scheduleCode, tripDate string) ([]domain.TripDetailsStageMetadata, error)
}

type orionmaxInventoryEvent struct {
	ActivityType string                  `json:"activity_type"`
	Data         []orionmaxInventoryItem `json:"data"`
	Zone         string                  `json:"zone"`
}

type orionmaxInventoryItem struct {
	ReferenceID  string `json:"refid"`
	DOJ          string `json:"doj"`
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	OperatorCode string `json:"operatorcode"`
	ScheduleCode string `json:"schedulecode"`
}

type inventoryRefreshJob struct {
	Metric  domain.QueueMetrix
	Payload []byte
}

func decodeInventoryRefreshEvent(activityType string, rawBody []byte) (string, orionmaxInventoryEvent, error) {
	actionType, exists := actionTypeByActivityType[activityType]
	if !exists {
		return "", orionmaxInventoryEvent{}, fmt.Errorf("%w: %q", ErrUnsupportedInventoryActivity, activityType)
	}
	var event orionmaxInventoryEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return "", orionmaxInventoryEvent{}, fmt.Errorf("decode Orionmax inventory event: %w", err)
	}
	if event.ActivityType != "" && event.ActivityType != activityType {
		return "", orionmaxInventoryEvent{}, fmt.Errorf("%w: URL=%q payload=%q", ErrInventoryActivityTypeMismatch, activityType, event.ActivityType)
	}
	if len(event.Data) == 0 {
		return "", orionmaxInventoryEvent{}, ErrInvalidInventoryEvent
	}
	return actionType, event, nil
}

func newQueueMetrix(activityType, actionType, zone string, item orionmaxInventoryItem, now time.Time) domain.QueueMetrix {
	return domain.QueueMetrix{
		ReferenceID: item.ReferenceID, ActivityType: activityType, ActionType: actionType,
		OperatorCode: item.OperatorCode, ScheduleCode: item.ScheduleCode, SourceStationCode: item.Source,
		DestinationStationCode: item.Destination, TripDate: metricTripDate(item.DOJ), Zone: zone,
		QueueStatus: domain.QueueStatusReceived, UpdatedAt: now,
	}
}

func metricTripDate(value string) string {
	date := strings.TrimSpace(value)
	if len(date) >= len("2006-01-02") {
		return date[:len("2006-01-02")]
	}
	return date
}

func buildInventoryRefreshJob(ctx context.Context, actionType string, item orionmaxInventoryItem, schedules InventoryScheduleReader, metric domain.QueueMetrix) (inventoryRefreshJob, error) {
	if item.ReferenceID == "" || item.OperatorCode == "" || item.Source == "" || item.Destination == "" {
		return inventoryRefreshJob{}, ErrInvalidInventoryEvent
	}
	tripDate, err := normalizeTripDate(item.DOJ)
	if err != nil {
		return inventoryRefreshJob{}, err
	}
	metric.TripDate = tripDate
	job := map[string]string{"referenceId": item.ReferenceID, "operatorCode": item.OperatorCode, "actionType": actionType}
	if actionType == "busmap" {
		if schedules == nil || item.ScheduleCode == "" || item.ScheduleCode == "NA" {
			return inventoryRefreshJob{}, ErrTripCodeUnavailable
		}
		candidates, err := schedules.FindStagesBySchedule(ctx, item.OperatorCode, item.ScheduleCode, tripDate)
		if err != nil {
			return inventoryRefreshJob{}, fmt.Errorf("find schedule metadata: %w", err)
		}
		tripCode, err := singleTripCode(candidates)
		if err != nil {
			return inventoryRefreshJob{}, err
		}
		metric.TripCode = tripCode
		job["tripCode"] = tripCode
		job["fromStationCode"] = item.Source
		job["toStationCode"] = item.Destination
		job["travelDate"] = tripDate
	} else {
		job["fromCode"] = item.Source
		job["toCode"] = item.Destination
		job["tripDate"] = tripDate
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return inventoryRefreshJob{}, fmt.Errorf("encode Worker refresh job: %w", err)
	}
	return inventoryRefreshJob{Metric: metric, Payload: payload}, nil
}

func normalizeTripDate(value string) (string, error) {
	date := metricTripDate(value)
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", ErrInvalidInventoryEvent
	}
	return date, nil
}

func singleTripCode(candidates []domain.TripDetailsStageMetadata) (string, error) {
	tripCodes := map[string]struct{}{}
	for _, candidate := range candidates {
		tripCodes[candidate.TripCode] = struct{}{}
	}
	if len(tripCodes) != 1 {
		return "", ErrTripCodeUnavailable
	}
	for tripCode := range tripCodes {
		return tripCode, nil
	}
	return "", fmt.Errorf("schedule metadata has no trip code")
}
