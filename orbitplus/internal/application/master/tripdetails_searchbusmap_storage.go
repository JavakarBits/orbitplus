package master

import (
	"context"
	"fmt"
)

// storeSearchBusMapEntries persists combined Search and BusMap Worker results.
func (persistence *TripDetailsStorage) storeSearchBusMapEntries(ctx context.Context, payload map[string]any, envelope, envelopeOperatorCode, suppliedActionType string) error {
	entries, _, _, err := extractEntries(payload, envelope, searchBusMapAction)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails SEARCHBUSMAP persistence started: envelope=%s actionType=%q entries=%d", envelope, suppliedActionType, len(entries))
	for index, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return errMissingStorageIdentifiers
		}
		operatorCode := envelopeOperatorCode
		if operatorCode == "" {
			operatorCode = stringField(entry, "operatorCode")
		}
		if operatorCode == "" {
			operatorCode = nestedStringField(entry, "operator", "code")
		}
		tripCode, tripStageCode := stringField(entry, "tripCode"), stringField(entry, "tripStageCode")
		tripDate := stringField(entry, "travelDate")
		fromCode, toCode := nestedStringField(entry, "fromStation", "code"), nestedStringField(entry, "toStation", "code")
		if err := requireKeyComponents(operatorCode, tripCode, tripStageCode, tripDate, fromCode, toCode); err != nil {
			return err
		}
		if err := persistence.storeSearchCache(ctx, operatorCode, tripCode, tripStageCode, entry, index); err != nil {
			return fmt.Errorf("store SEARCHBUSMAP Search cache: %w", err)
		}
		if err := persistence.storeSearchMetadata(ctx, operatorCode, tripCode, tripStageCode, tripDate, fromCode, toCode, entry, index); err != nil {
			return fmt.Errorf("store SEARCHBUSMAP metadata: %w", err)
		}
		if err := persistence.storeBusMapCache(ctx, operatorCode, tripCode, tripStageCode, entry, index); err != nil {
			return fmt.Errorf("store SEARCHBUSMAP BusMap cache: %w", err)
		}
	}
	persistence.logger.Printf("TripDetails SEARCHBUSMAP persistence completed: entries=%d", len(entries))
	return nil
}
