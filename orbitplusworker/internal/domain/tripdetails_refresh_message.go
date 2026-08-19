// Package domain contains immutable Worker message identities and validation.
package domain

import (
	"fmt"
	"strings"
)

// TripDetailsRefreshMessage is the durable RabbitMQ payload for one direct
// TripDetails refresh. Source credentials are deliberately excluded.
type TripDetailsRefreshMessage struct {
	ActionType      string `json:"actionType"`
	ReferenceID     string `json:"referenceId"`
	OperatorCode    string `json:"operatorCode"`
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

	switch message.ActionType {
	case ActionSearch, ActionSearchBusMap:
		return requireFields(message.ActionType,
			field{name: "fromCode", value: message.FromCode},
			field{name: "toCode", value: message.ToCode},
			field{name: "tripDate", value: message.TripDate},
		)
	case ActionBusMap:
		return requireFields(message.ActionType,
			field{name: "tripCode", value: message.TripCode},
			field{name: "fromStationCode", value: message.FromStationCode},
			field{name: "toStationCode", value: message.ToStationCode},
			field{name: "travelDate", value: message.TravelDate},
		)
	default:
		return fmt.Errorf("unsupported actionType: %s", message.ActionType)
	}
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
