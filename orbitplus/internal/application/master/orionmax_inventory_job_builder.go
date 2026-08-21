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
	ErrInvalidInventoryEvent         = errors.New("invalid inventory event")
	ErrTripCodeUnavailable           = errors.New("trip code unavailable for schedule")
	ErrZoneURLUnavailable            = errors.New("zone URL is unavailable")
)

var actionTypeByActivityType = map[string]string{
	"daily-job": "searchbusmap", "ticket-tentative-block-cancel": "searchbusmap", "block-release": "searchbusmap",
	"quick-fare-change": "searchbusmap", "dp-fare-change": "searchbusmap", "seat-block-release": "searchbusmap",
	"ticket-cancel": "searchbusmap", "manual-refresh": "searchbusmap", "notify-schedule": "searchbusmap",
	"fare-change": "searchbusmap", "phone-ticket-cancel": "searchbusmap", "not-travel": "searchbusmap",
	"service_inactive": "searchbusmap", "vsd_toggle": "searchbusmap", "no_show": "searchbusmap", "shift": "searchbusmap",
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
	TripCode     string `json:"tripcode"`
}

type inventoryRefreshJob struct {
	Metric  domain.QueueMetrix
	Payload []byte
}

func decodeInventoryRefreshEvent(activityType string, rawBody []byte) (string, orionmaxInventoryEvent, error) {
	actionType := actionTypeByActivityType[activityType]
	if actionType == "" {
		actionType = "searchbusmap"
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
		OperatorCode: item.OperatorCode, ScheduleCode: item.ScheduleCode, TripCode: strings.TrimSpace(item.TripCode),
		SourceStationCode:      item.Source,
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
	zoneURL, exists := zoneURLFor(metric.Zone)
	if !exists {
		return inventoryRefreshJob{}, fmt.Errorf("%w: %q", ErrZoneURLUnavailable, metric.Zone)
	}
	job := map[string]string{
		"referenceId": item.ReferenceID, "operatorCode": item.OperatorCode, "actionType": actionType, "zoneURL": zoneURL,
	}
	tripCode, err := resolveInventoryTripCode(ctx, item, schedules, tripDate)
	if err != nil {
		tripCode = ""
	}
	if tripCode != "" {
		metric.TripCode = tripCode
		job["tripCode"] = tripCode
	}
	if actionType == "busmap" {
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
	metric.WorkerPayload = append(json.RawMessage(nil), payload...)
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

// resolveInventoryTripCode prefers an Orionmax-supplied trip code and otherwise
// resolves the code from the schedule metadata when a usable schedule is present.
func resolveInventoryTripCode(ctx context.Context, item orionmaxInventoryItem, schedules InventoryScheduleReader, tripDate string) (string, error) {
	if tripCode := strings.TrimSpace(item.TripCode); tripCode != "" {
		return tripCode, nil
	}
	if schedules == nil || item.ScheduleCode == "" || strings.EqualFold(item.ScheduleCode, "NA") {
		return "", nil
	}
	candidates, err := schedules.FindStagesBySchedule(ctx, item.OperatorCode, item.ScheduleCode, tripDate)
	if err != nil {
		return "", fmt.Errorf("find schedule metadata: %w", err)
	}
	tripCode, err := singleTripCode(candidates)
	if err != nil {
		return "", err
	}
	return tripCode, nil
}
