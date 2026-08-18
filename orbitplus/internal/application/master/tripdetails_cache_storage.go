package master

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TripDetailsCacheRepository stores JSON content in Dragonfly without exposing
// cache-client details to the application layer.
type TripDetailsCacheRepository interface {
	SetJSON(ctx context.Context, key string, value []byte) error
}

// BuildTripKey returns the Dragonfly key for common Search Trip content.
func BuildTripKey(operatorCode, tripCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode); err != nil {
		return "", err
	}
	return "trip:" + operatorCode + ":" + tripCode, nil
}

// BuildStageKey returns the Dragonfly key for Search Stage content.
func BuildStageKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "stage:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

// BuildBusMapKey returns the Dragonfly key for a complete BUSMAP response.
func BuildBusMapKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "busmap:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

func requireKeyComponents(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.Contains(value, ":") {
			return errMissingStorageIdentifiers
		}
	}
	return nil
}

func (persistence *TripDetailsStorage) storeSearchCache(ctx context.Context, operatorCode, tripCode, tripStageCode string, entry map[string]any, index int) error {
	tripKey, err := BuildTripKey(operatorCode, tripCode)
	if err != nil {
		return err
	}
	stageKey, err := BuildStageKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails Dragonfly SEARCH keys entry=%d trip=%q stage=%q", index, tripKey, stageKey)
	tripJSON, err := json.Marshal(commonTripProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal common Trip JSON: %w", err)
	}
	stageJSON, err := json.Marshal(stageProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal Stage JSON: %w", err)
	}
	if err := persistence.cache.SetJSON(ctx, tripKey, tripJSON); err != nil {
		return fmt.Errorf("store common Trip content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=trip key=%q", index, tripKey)
	if err := persistence.cache.SetJSON(ctx, stageKey, stageJSON); err != nil {
		return fmt.Errorf("store Stage content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=stage key=%q", index, stageKey)
	return nil
}

func (persistence *TripDetailsStorage) storeBusMapCache(ctx context.Context, operatorCode, tripCode, tripStageCode string, entry map[string]any, index int) error {
	busMapKey, err := BuildBusMapKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails BUSMAP persistence entry=%d identifiers operatorCode=%q tripCode=%q tripStageCode=%q", index, operatorCode, tripCode, tripStageCode)
	busMapJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal BUSMAP JSON: %w", err)
	}
	if err := persistence.cache.SetJSON(ctx, busMapKey, busMapJSON); err != nil {
		return fmt.Errorf("store BUSMAP content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=busmap key=%q", index, busMapKey)
	return nil
}

var searchStageFields = map[string]struct{}{
	"tripStageCode": {}, "fromStation": {}, "toStation": {}, "stationPoint": {}, "stageFare": {},
}

func commonTripProjection(entry map[string]any) map[string]any {
	common := make(map[string]any, len(entry))
	for key, value := range entry {
		if _, isStageField := searchStageFields[key]; isStageField {
			continue
		}
		if key == "bus" {
			if bus, ok := value.(map[string]any); ok {
				common[key] = copyWithoutSeatLayoutList(bus)
				continue
			}
		}
		common[key] = value
	}
	return common
}

func stageProjection(entry map[string]any) map[string]any {
	stage := make(map[string]any, len(searchStageFields))
	for key := range searchStageFields {
		if value, exists := entry[key]; exists {
			stage[key] = value
		}
	}
	return stage
}

func copyWithoutSeatLayoutList(bus map[string]any) map[string]any {
	copy := make(map[string]any, len(bus))
	for key, value := range bus {
		if key != "seatLayoutList" {
			copy[key] = value
		}
	}
	return copy
}
