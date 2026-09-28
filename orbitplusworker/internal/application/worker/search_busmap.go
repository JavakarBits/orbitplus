package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"orbitplusworker/internal/domain"
)

// searchBusMapIdentifiers are taken from one Search result and identify the
// concrete BusMap request for that trip stage.
type searchBusMapIdentifiers struct {
	tripCode      string
	tripStageCode string
	travelDate    string
	fromCode      string
	toCode        string
}

// fetchSearchAndBusMaps implements searchbusmap as a Worker workflow, not a
// BITS endpoint. It calls the real Search endpoint first, then the real BusMap
// endpoint once for every Search entry. Nothing is submitted to OrbitPlus until
// all BusMap calls succeed, so the existing retry boundary remains the complete
// operation rather than a partially persisted result.
func (worker *TripDetailsRefreshWorker) fetchSearchAndBusMaps(ctx context.Context, message domain.TripDetailsRefreshMessage, credential BitsOperatorCredential) (BitsTripDetailsResponse, error) {
	searchMessage := message
	searchMessage.ActionType = domain.ActionSearch
	searchResponse, err := worker.source.FetchTripDetails(ctx, BitsTripDetailsRequest{Message: searchMessage, Credential: credential})
	if err != nil {
		return BitsTripDetailsResponse{}, err
	}
	if len(searchResponse.Body) == 0 {
		return BitsTripDetailsResponse{}, fmt.Errorf("Bits Search response is empty")
	}

	envelope, searchEntries, err := decodeSearchResponse(searchResponse.Body)
	if err != nil {
		return BitsTripDetailsResponse{}, err
	}
	combinedEntries := make([]any, 0, len(searchEntries))
	for index, searchEntry := range searchEntries {
		identifiers, err := searchEntryIdentifiers(searchEntry)
		if err != nil {
			return BitsTripDetailsResponse{}, fmt.Errorf("Bits Search entry %d is invalid: %w", index, err)
		}

		// The delivery-level rate-limit slot acquired by Handle covers Search.
		// Every additional BusMap GET consumes its own slot so a fan-out cannot
		// bypass the per-zone request quota.
		if err := worker.awaitZoneRateLimit(ctx, message); err != nil {
			return BitsTripDetailsResponse{}, err
		}
		busMapMessage := message
		busMapMessage.ActionType = domain.ActionBusMap
		busMapMessage.TripCode = identifiers.tripCode
		busMapMessage.FromStationCode = identifiers.fromCode
		busMapMessage.ToStationCode = identifiers.toCode
		busMapMessage.TravelDate = identifiers.travelDate

		busMapResponse, err := worker.source.FetchTripDetails(ctx, BitsTripDetailsRequest{Message: busMapMessage, Credential: credential})
		if err != nil {
			return BitsTripDetailsResponse{}, err
		}
		if len(busMapResponse.Body) == 0 {
			return BitsTripDetailsResponse{}, fmt.Errorf("Bits BusMap response is empty for Search entry %d", index)
		}
		busMapEntry, err := decodeBusMapResponse(busMapResponse.Body)
		if err != nil {
			return BitsTripDetailsResponse{}, fmt.Errorf("Bits BusMap response for Search entry %d is invalid: %w", index, err)
		}
		if err := validateBusMapIdentity(busMapEntry, identifiers); err != nil {
			return BitsTripDetailsResponse{}, fmt.Errorf("Bits BusMap response for Search entry %d does not match Search: %w", index, err)
		}

		combined := mergeJSONObjects(searchEntry, busMapEntry)
		// Search is the routing authority. Preserve its identifiers even when a
		// BusMap response omits them; conflicting non-empty BusMap identifiers
		// were rejected above.
		combined["tripCode"] = identifiers.tripCode
		combined["tripStageCode"] = identifiers.tripStageCode
		combined["travelDate"] = identifiers.travelDate
		setNestedCode(combined, "fromStation", identifiers.fromCode)
		setNestedCode(combined, "toStation", identifiers.toCode)
		combinedEntries = append(combinedEntries, combined)
	}

	combinedEnvelope := cloneJSONObject(envelope)
	combinedEnvelope["data"] = combinedEntries
	body, err := json.Marshal(combinedEnvelope)
	if err != nil {
		return BitsTripDetailsResponse{}, fmt.Errorf("encode combined Search and BusMap response: %w", err)
	}
	return BitsTripDetailsResponse{Body: body}, nil
}

func decodeSearchResponse(body []byte) (map[string]any, []map[string]any, error) {
	envelope, err := decodeJSONObject(body)
	if err != nil {
		return nil, nil, fmt.Errorf("decode Bits Search response: %w", err)
	}
	if err := requireSuccessfulBitsStatus(envelope); err != nil {
		return nil, nil, err
	}
	rawEntries, ok := envelope["data"].([]any)
	if !ok || len(rawEntries) == 0 {
		return nil, nil, fmt.Errorf("Bits Search response has no data entries")
	}
	entries := make([]map[string]any, 0, len(rawEntries))
	for index, rawEntry := range rawEntries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("Bits Search data entry %d is not an object", index)
		}
		entries = append(entries, entry)
	}
	return envelope, entries, nil
}

func decodeBusMapResponse(body []byte) (map[string]any, error) {
	envelope, err := decodeJSONObject(body)
	if err != nil {
		return nil, fmt.Errorf("decode Bits BusMap response: %w", err)
	}
	if err := requireSuccessfulBitsStatus(envelope); err != nil {
		return nil, err
	}
	if rawData, exists := envelope["data"]; exists {
		switch data := rawData.(type) {
		case map[string]any:
			if len(data) == 0 {
				return nil, fmt.Errorf("Bits BusMap response data is empty")
			}
			return data, nil
		case []any:
			if len(data) != 1 {
				return nil, fmt.Errorf("Bits BusMap response must contain exactly one data entry")
			}
			entry, ok := data[0].(map[string]any)
			if !ok || len(entry) == 0 {
				return nil, fmt.Errorf("Bits BusMap response data entry is invalid")
			}
			return entry, nil
		default:
			return nil, fmt.Errorf("Bits BusMap response data is invalid")
		}
	}
	// Some BusMap deployments return the entry directly rather than wrapping it
	// in data. Accept that only when it has a trip identity.
	if textField(envelope, "tripCode") == "" && textField(envelope, "tripStageCode") == "" {
		return nil, fmt.Errorf("Bits BusMap response has no data entry")
	}
	entry := cloneJSONObject(envelope)
	delete(entry, "status")
	return entry, nil
}

func decodeJSONObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("response is not a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("response contains more than one JSON value")
		}
		return nil, err
	}
	return object, nil
}

func requireSuccessfulBitsStatus(object map[string]any) error {
	status, exists := object["status"]
	if !exists {
		return nil
	}
	switch value := status.(type) {
	case json.Number:
		if value.String() == "1" {
			return nil
		}
	case float64:
		if value == 1 {
			return nil
		}
	}
	return fmt.Errorf("Bits returned unsuccessful status")
}

func searchEntryIdentifiers(entry map[string]any) (searchBusMapIdentifiers, error) {
	identifiers := searchBusMapIdentifiers{
		tripCode:      textField(entry, "tripCode"),
		tripStageCode: textField(entry, "tripStageCode"),
		travelDate:    textField(entry, "travelDate"),
		fromCode:      nestedTextField(entry, "fromStation", "code"),
		toCode:        nestedTextField(entry, "toStation", "code"),
	}
	for name, value := range map[string]string{
		"tripCode": identifiers.tripCode, "tripStageCode": identifiers.tripStageCode,
		"travelDate": identifiers.travelDate, "fromStation.code": identifiers.fromCode,
		"toStation.code": identifiers.toCode,
	} {
		if value == "" {
			return searchBusMapIdentifiers{}, fmt.Errorf("missing %s", name)
		}
	}
	return identifiers, nil
}

func validateBusMapIdentity(entry map[string]any, expected searchBusMapIdentifiers) error {
	checks := []struct {
		name     string
		actual   string
		expected string
	}{
		{name: "tripCode", actual: textField(entry, "tripCode"), expected: expected.tripCode},
		{name: "tripStageCode", actual: textField(entry, "tripStageCode"), expected: expected.tripStageCode},
		{name: "travelDate", actual: textField(entry, "travelDate"), expected: expected.travelDate},
		{name: "fromStation.code", actual: nestedTextField(entry, "fromStation", "code"), expected: expected.fromCode},
		{name: "toStation.code", actual: nestedTextField(entry, "toStation", "code"), expected: expected.toCode},
	}
	for _, check := range checks {
		if check.actual != "" && check.actual != check.expected {
			return fmt.Errorf("%s is %q, expected %q", check.name, check.actual, check.expected)
		}
	}
	return nil
}

func textField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func nestedTextField(object map[string]any, parent, key string) string {
	nested, _ := object[parent].(map[string]any)
	return textField(nested, key)
}

func setNestedCode(object map[string]any, field, code string) {
	nested, _ := object[field].(map[string]any)
	nested = cloneJSONObject(nested)
	nested["code"] = code
	object[field] = nested
}

func mergeJSONObjects(base, overlay map[string]any) map[string]any {
	result := cloneJSONObject(base)
	for key, overlayValue := range overlay {
		baseObject, baseIsObject := result[key].(map[string]any)
		overlayObject, overlayIsObject := overlayValue.(map[string]any)
		if baseIsObject && overlayIsObject {
			result[key] = mergeJSONObjects(baseObject, overlayObject)
			continue
		}
		result[key] = cloneJSONValue(overlayValue)
	}
	return result
}

func cloneJSONObject(object map[string]any) map[string]any {
	result := make(map[string]any, len(object))
	for key, value := range object {
		result[key] = cloneJSONValue(value)
	}
	return result
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONObject(typed)
	case []any:
		result := make([]any, len(typed))
		for index, entry := range typed {
			result[index] = cloneJSONValue(entry)
		}
		return result
	default:
		return value
	}
}
