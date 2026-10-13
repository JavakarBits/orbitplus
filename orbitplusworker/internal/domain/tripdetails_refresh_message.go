// Package domain contains immutable Worker message identities and validation.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// TripDetailsRefreshMessage is the durable RabbitMQ payload for one direct
// TripDetails refresh. Source credentials are deliberately excluded.
type TripDetailsRefreshMessage struct {
	ActionType      string `json:"actionType"`
	ReferenceID     string `json:"referenceId"`
	OperatorCode    string `json:"operatorCode"`
	ZoneURL         string `json:"zoneURL"`
	FromCode        string `json:"fromCode"`
	ToCode          string `json:"toCode"`
	TripDate        string `json:"tripDate"`
	TripCode        string `json:"tripCode"`
	FromStationCode string `json:"fromStationCode"`
	ToStationCode   string `json:"toStationCode"`
	TravelDate      string `json:"travelDate"`
}

const (
	ActionSearch       = "search"
	ActionBusMap       = "busmap"
	ActionSearchBusMap = "searchbusmap"
)

func (message TripDetailsRefreshMessage) Validate() error {
	if strings.TrimSpace(message.ActionType) == "" {
		return fmt.Errorf("missing actionType")
	}
	if strings.TrimSpace(message.OperatorCode) == "" {
		return fmt.Errorf("missing operatorCode")
	}
	if strings.TrimSpace(message.ZoneURL) == "" {
		return fmt.Errorf("missing zoneURL")
	}

	switch message.ActionType {
	case ActionSearch, ActionSearchBusMap:
		if err := requireFields(message.ActionType,
			field{name: "fromCode", value: message.FromCode},
			field{name: "toCode", value: message.ToCode},
			field{name: "tripDate", value: message.TripDate},
		); err != nil {
			return err
		}
		return validateCurrentOrFutureDate("tripDate", message.TripDate)
	case ActionBusMap:
		if err := requireFields(message.ActionType,
			field{name: "tripCode", value: message.TripCode},
			field{name: "fromStationCode", value: message.FromStationCode},
			field{name: "toStationCode", value: message.ToStationCode},
			field{name: "travelDate", value: message.TravelDate},
		); err != nil {
			return err
		}
		return validateCurrentOrFutureDate("travelDate", message.TravelDate)
	default:
		return fmt.Errorf("unsupported actionType: %s", message.ActionType)
	}
}

const tripDateLayout = "2006-01-02"

func validateCurrentOrFutureDate(name, value string) error {
	value = strings.TrimSpace(value)
	date, err := time.Parse(tripDateLayout, value)
	if err != nil {
		return fmt.Errorf("%s must use YYYY-MM-DD format", name)
	}

	// Compare calendar dates rather than timestamps so today's trips remain
	// valid regardless of the current time of day.
	today, _ := time.Parse(tripDateLayout, time.Now().Format(tripDateLayout))
	if date.Before(today) {
		return fmt.Errorf("%s cannot be in the past", name)
	}
	return nil
}

type field struct {
	name  string
	value string
}

func requireFields(actionType string, fields ...field) error {
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("missing %s for actionType: %s", field.name, actionType)
		}
	}
	return nil
}
