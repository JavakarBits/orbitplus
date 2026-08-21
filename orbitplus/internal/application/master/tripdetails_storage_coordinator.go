package master

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
)

var errMissingStorageIdentifiers = errors.New("TripDetails payload is missing required persistence identifiers")

// TripDetailsStorage coordinates Search and BUSMAP persistence.
type TripDetailsStorage struct {
	cache    TripDetailsCacheRepository
	metadata TripDetailsMetadataRepository
	logger   *log.Logger
	now      func() time.Time
}

// NewTripDetailsStorage constructs TripDetails persistence dependencies.
func NewTripDetailsStorage(cache TripDetailsCacheRepository, metadata TripDetailsMetadataRepository) *TripDetailsStorage {
	return NewTripDetailsStorageWithLogger(cache, metadata, log.Default())
}

// NewTripDetailsStorageWithLogger constructs persistence with a caller-supplied logger.
func NewTripDetailsStorageWithLogger(cache TripDetailsCacheRepository, metadata TripDetailsMetadataRepository, logger *log.Logger) *TripDetailsStorage {
	return &TripDetailsStorage{cache: cache, metadata: metadata, logger: logger, now: time.Now}
}

const (
	workerEnvelope     = "worker"
	directBitsEnvelope = "direct_bits"
	busMapAction       = "BUSMAP"
	searchBusMapAction = "SEARCHBUSMAP"
)

type tripDetailsStoreResult struct {
	UpdatedTripCodes []string
}

// Store stores valid Search and BUSMAP entries and reports each trip code whose
// complete entry was persisted.
func (persistence *TripDetailsStorage) Store(ctx context.Context, value any) (tripDetailsStoreResult, error) {
	result := tripDetailsStoreResult{}
	if persistence == nil || persistence.cache == nil || persistence.metadata == nil {
		return result, errors.New("TripDetails persistence is not configured")
	}

	payload, suppliedActionType, operatorCode, envelope, err := resolveTripDetailsPayload(value)
	if err != nil {
		persistence.logger.Printf("TripDetails persistence extraction failed: %v", err)
		return result, err
	}

	actionType := strings.ToUpper(suppliedActionType)
	if actionType == searchBusMapAction {
		result.UpdatedTripCodes, err = persistence.storeSearchBusMapEntries(ctx, payload, envelope, operatorCode, suppliedActionType)
		return result, err
	}

	entries, actionType, inferredBusMap, err := extractEntries(payload, envelope, actionType)
	if err != nil {
		persistence.logger.Printf("TripDetails persistence extraction failed: envelope=%s actionType=%q reason=missing_data_entries", envelope, suppliedActionType)
		return result, err
	}
	if inferredBusMap {
		persistence.logger.Printf("TripDetails persistence inferred normalizedActionType=%q from direct single-entry response", actionType)
	}

	persistence.logger.Printf("TripDetails persistence started: envelope=%s actionType=%q normalizedActionType=%q entries=%d", envelope, suppliedActionType, actionType, len(entries))
	result.UpdatedTripCodes, err = persistence.storeEntries(ctx, entries, actionType, operatorCode, suppliedActionType)
	if err != nil {
		return result, err
	}
	persistence.logger.Printf("TripDetails persistence completed: envelope=%s actionType=%q entries=%d", envelope, suppliedActionType, len(entries))
	return result, nil
}

func extractEntries(payload map[string]any, envelope, actionType string) ([]any, string, bool, error) {
	if entries, isArray := payload["data"].([]any); isArray {
		if len(entries) == 0 {
			return nil, actionType, false, errMissingStorageIdentifiers
		}
		return entries, actionType, false, nil
	}

	if entry, isObject := payload["data"].(map[string]any); isObject && acceptsSingleDataEntry(envelope, actionType) {
		actionType, inferredBusMap := inferBusMapAction(envelope, actionType)
		return []any{entry}, actionType, inferredBusMap, nil
	}

	if isWorkerRootBusMap(envelope, actionType, payload) {
		return []any{payload}, actionType, false, nil
	}

	return nil, actionType, false, errMissingStorageIdentifiers
}

func acceptsSingleDataEntry(envelope, actionType string) bool {
	return envelope == directBitsEnvelope || envelope == workerEnvelope && actionType == busMapAction
}

func inferBusMapAction(envelope, actionType string) (string, bool) {
	if envelope == directBitsEnvelope && actionType == "" {
		return busMapAction, true
	}
	return actionType, false
}

func isWorkerRootBusMap(envelope, actionType string, payload map[string]any) bool {
	if envelope != workerEnvelope || actionType != busMapAction {
		return false
	}
	_, hasData := payload["data"]
	return !hasData
}

func (persistence *TripDetailsStorage) storeEntries(ctx context.Context, entries []any, actionType, operatorCode, suppliedActionType string) ([]string, error) {
	tripCodes := make([]string, 0, len(entries))
	for index, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			persistence.logger.Printf("TripDetails persistence extraction failed: entry=%d reason=non_object_entry", index)
			return tripCodes, errMissingStorageIdentifiers
		}
		if err := persistence.storeEntry(ctx, actionType, operatorCode, entry, index); err != nil {
			persistence.logger.Printf("TripDetails persistence failed: entry=%d actionType=%q error=%v", index, suppliedActionType, err)
			return tripCodes, err
		}
		tripCodes = appendUpdatedTripCode(tripCodes, stringField(entry, "tripCode"))
	}
	return tripCodes, nil
}

func appendUpdatedTripCode(tripCodes []string, tripCode string) []string {
	tripCode = strings.TrimSpace(tripCode)
	if tripCode == "" {
		return tripCodes
	}
	for _, existing := range tripCodes {
		if existing == tripCode {
			return tripCodes
		}
	}
	return append(tripCodes, tripCode)
}

func (persistence *TripDetailsStorage) storeEntry(ctx context.Context, actionType, envelopeOperatorCode string, entry map[string]any, index int) error {
	operatorCode := envelopeOperatorCode
	if operatorCode == "" {
		operatorCode = stringField(entry, "operatorCode")
	}
	if operatorCode == "" {
		operatorCode = nestedStringField(entry, "operator", "code")
	}
	tripCode := stringField(entry, "tripCode")
	tripStageCode := stringField(entry, "tripStageCode")

	if actionType == busMapAction {
		return persistence.storeBusMapCache(ctx, operatorCode, tripCode, tripStageCode, entry, index)
	}

	tripDate := stringField(entry, "travelDate")
	fromStationCode := nestedStringField(entry, "fromStation", "code")
	toStationCode := nestedStringField(entry, "toStation", "code")
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode, tripDate, fromStationCode, toStationCode); err != nil {
		return err
	}
	if err := persistence.storeSearchCache(ctx, operatorCode, tripCode, tripStageCode, entry, index); err != nil {
		return err
	}
	return persistence.storeSearchMetadata(ctx, operatorCode, tripCode, tripStageCode, tripDate, fromStationCode, toStationCode, entry, index)
}

func resolveTripDetailsPayload(value any) (map[string]any, string, string, string, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, "", "", "unknown", errMissingStorageIdentifiers
	}
	actionType := stringField(root, "actionType")
	operatorCode := stringField(root, "operatorCode")
	payload := root
	envelope := directBitsEnvelope
	if orbitResponse, exists := root["orbitResponse"]; exists {
		var isObject bool
		payload, isObject = orbitResponse.(map[string]any)
		if !isObject {
			return nil, "", "", workerEnvelope, errMissingStorageIdentifiers
		}
		envelope = workerEnvelope
	}
	if actionType == "" {
		actionType = stringField(payload, "actionType")
	}
	if operatorCode == "" {
		operatorCode = stringField(payload, "operatorCode")
	}
	return payload, actionType, operatorCode, envelope, nil
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func nestedStringField(object map[string]any, parent, key string) string {
	nested, _ := object[parent].(map[string]any)
	return stringField(nested, key)
}
