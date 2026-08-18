package master

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"orbitplusmaster/internal/domain"
)

var errMissingPersistenceIdentifiers = errors.New("TripDetails payload is missing required persistence identifiers")

// TripDetailsCacheRepository stores JSON content in Dragonfly without exposing
// cache-client details to the application layer.
type TripDetailsCacheRepository interface {
	SetJSON(ctx context.Context, key string, value []byte) error
}

// TripDetailsMetadataRepository stores only Stage lookup metadata in Cassandra.
type TripDetailsMetadataRepository interface {
	SaveStageMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error
}

// TripDetailsPersistence coordinates the Phase 2 content and metadata writes.
type TripDetailsPersistence struct {
	cache    TripDetailsCacheRepository
	metadata TripDetailsMetadataRepository
	logger   *log.Logger
	now      func() time.Time
}

// NewTripDetailsPersistence constructs Phase 2 persistence dependencies.
func NewTripDetailsPersistence(cache TripDetailsCacheRepository, metadata TripDetailsMetadataRepository) *TripDetailsPersistence {
	return NewTripDetailsPersistenceWithLogger(cache, metadata, log.Default())
}

// NewTripDetailsPersistenceWithLogger constructs persistence with a caller-supplied logger.
func NewTripDetailsPersistenceWithLogger(cache TripDetailsCacheRepository, metadata TripDetailsMetadataRepository, logger *log.Logger) *TripDetailsPersistence {
	return &TripDetailsPersistence{cache: cache, metadata: metadata, logger: logger, now: time.Now}
}

// BuildTripKey returns the centralized Dragonfly key for common Trip content.
func BuildTripKey(operatorCode, tripCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode); err != nil {
		return "", err
	}
	return "trip:" + operatorCode + ":" + tripCode, nil
}

// BuildStageKey returns the centralized Dragonfly key for one complete Stage.
func BuildStageKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "stage:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

// BuildBusMapKey returns the centralized Dragonfly key for BUSMAP content.
func BuildBusMapKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "busmap:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

func requireKeyComponents(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.Contains(value, ":") {
			return errMissingPersistenceIdentifiers
		}
	}
	return nil
}

const (
	workerEnvelope     = "worker"
	directBitsEnvelope = "direct_bits"
	busMapAction       = "BUSMAP"
	searchBusMapAction = "SEARCHBUSMAP"
)

// Persist stores lossless Trip and Stage content for cacheable entries. BUSMAP
// seat layout is written only for BUSMAP; SEARCHBUSMAP remains an approved TODO.
func (persistence *TripDetailsPersistence) Persist(ctx context.Context, value any) error {
	if persistence == nil || persistence.cache == nil || persistence.metadata == nil {
		return errors.New("TripDetails persistence is not configured")
	}

	payload, suppliedActionType, operatorCode, envelope, err := resolveTripDetailsPayload(value)
	if err != nil {
		persistence.logger.Printf("TripDetails persistence extraction failed: %v", err)
		return err
	}

	actionType := strings.ToUpper(suppliedActionType)
	if actionType == searchBusMapAction {
		persistence.logger.Printf("TripDetails persistence skipped: envelope=%s actionType=%q normalizedActionType=%q reason=SEARCHBUSMAP_TODO", envelope, suppliedActionType, actionType)
		return nil
	}

	entries, actionType, inferredBusMap, err := extractPersistenceEntries(payload, envelope, actionType)
	if err != nil {
		persistence.logger.Printf("TripDetails persistence extraction failed: envelope=%s actionType=%q reason=missing_data_entries", envelope, suppliedActionType)
		return err
	}
	if inferredBusMap {
		persistence.logger.Printf("TripDetails persistence inferred normalizedActionType=%q from direct single-entry response", actionType)
	}

	persistence.logger.Printf("TripDetails persistence started: envelope=%s actionType=%q normalizedActionType=%q entries=%d", envelope, suppliedActionType, actionType, len(entries))
	if err := persistence.persistEntries(ctx, entries, actionType, operatorCode, suppliedActionType); err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails persistence completed: envelope=%s actionType=%q entries=%d", envelope, suppliedActionType, len(entries))
	return nil
}

func extractPersistenceEntries(payload map[string]any, envelope, actionType string) ([]any, string, bool, error) {
	if entries, isArray := payload["data"].([]any); isArray {
		if len(entries) == 0 {
			return nil, actionType, false, errMissingPersistenceIdentifiers
		}
		return entries, actionType, false, nil
	}

	if entry, isObject := payload["data"].(map[string]any); isObject && acceptsSingleDataEntry(envelope, actionType) {
		actionType, inferredBusMap := inferBusMapAction(envelope, actionType, entry)
		return []any{entry}, actionType, inferredBusMap, nil
	}

	if isWorkerRootBusMap(envelope, actionType, payload) {
		return []any{payload}, actionType, false, nil
	}

	return nil, actionType, false, errMissingPersistenceIdentifiers
}

func acceptsSingleDataEntry(envelope, actionType string) bool {
	return envelope == directBitsEnvelope || envelope == workerEnvelope && actionType == busMapAction
}

func inferBusMapAction(envelope, actionType string, _ map[string]any) (string, bool) {
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

func (persistence *TripDetailsPersistence) persistEntries(ctx context.Context, entries []any, actionType, operatorCode, suppliedActionType string) error {
	for index, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			persistence.logger.Printf("TripDetails persistence extraction failed: entry=%d reason=non_object_entry", index)
			return errMissingPersistenceIdentifiers
		}
		if err := persistence.persistEntry(ctx, actionType, operatorCode, entry, index); err != nil {
			persistence.logger.Printf("TripDetails persistence failed: entry=%d actionType=%q error=%v", index, suppliedActionType, err)
			return err
		}
	}
	return nil
}

func (persistence *TripDetailsPersistence) persistEntry(ctx context.Context, actionType, envelopeOperatorCode string, entry map[string]any, index int) error {
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
		return persistence.persistBusMapEntry(ctx, operatorCode, tripCode, tripStageCode, entry, index)
	}

	travelDate := stringField(entry, "travelDate")
	fromStationCode := nestedStringField(entry, "fromStation", "code")
	toStationCode := nestedStringField(entry, "toStation", "code")
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode, travelDate, fromStationCode, toStationCode); err != nil {
		return err
	}

	tripKey, err := BuildTripKey(operatorCode, tripCode)
	if err != nil {
		return err
	}
	stageKey, err := BuildStageKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails SEARCH persistence entry=%d identifiers operatorCode=%q tripCode=%q tripStageCode=%q travelDate=%q fromStationCode=%q toStationCode=%q", index, operatorCode, tripCode, tripStageCode, travelDate, fromStationCode, toStationCode)
	persistence.logger.Printf("TripDetails Dragonfly SEARCH keys entry=%d trip=%q stage=%q", index, tripKey, stageKey)
	commonJSON, err := json.Marshal(commonTripProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal common Trip JSON: %w", err)
	}
	stageJSON, err := json.Marshal(stageProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal Stage JSON: %w", err)
	}
	if err := persistence.cache.SetJSON(ctx, tripKey, commonJSON); err != nil {
		return fmt.Errorf("store common Trip content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=trip key=%q", index, tripKey)
	if err := persistence.cache.SetJSON(ctx, stageKey, stageJSON); err != nil {
		return fmt.Errorf("store Stage content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=stage key=%q", index, stageKey)

	metadata := domain.TripDetailsStageMetadata{
		OperatorCode: operatorCode, TripCode: tripCode, TravelDate: travelDate,
		FromStationCode: fromStationCode, ToStationCode: toStationCode,
		TripStageCode: tripStageCode, UpdatedAt: persistence.now().UTC(),
	}
	for _, record := range stageMetadataRecords(metadata, entry) {
		if err := persistence.metadata.SaveStageMetadata(ctx, record); err != nil {
			return fmt.Errorf("store TripDetails metadata: %w", err)
		}
	}
	persistence.logger.Printf("TripDetails Cassandra metadata write succeeded entry=%d tripStageCode=%q", index, tripStageCode)
	return nil
}

// persistBusMapEntry stores the complete BUSMAP response separately from
// SEARCH-owned Trip, Stage, and Cassandra metadata records.
func (persistence *TripDetailsPersistence) persistBusMapEntry(ctx context.Context, operatorCode, tripCode, tripStageCode string, entry map[string]any, index int) error {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return err
	}
	busMapKey, err := BuildBusMapKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails BUSMAP persistence entry=%d identifiers operatorCode=%q tripCode=%q tripStageCode=%q", index, operatorCode, tripCode, tripStageCode)
	persistence.logger.Printf("TripDetails Dragonfly BUSMAP key entry=%d key=%q", index, busMapKey)
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

func resolveTripDetailsPayload(value any) (map[string]any, string, string, string, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, "", "", "unknown", errMissingPersistenceIdentifiers
	}
	actionType := stringField(root, "actionType")
	operatorCode := stringField(root, "operatorCode")
	payload := root
	envelope := "direct_bits"
	if orbitResponse, exists := root["orbitResponse"]; exists {
		var isObject bool
		payload, isObject = orbitResponse.(map[string]any)
		if !isObject {
			return nil, "", "", "worker", errMissingPersistenceIdentifiers
		}
		envelope = "worker"
	}
	if actionType == "" {
		actionType = stringField(payload, "actionType")
	}
	if operatorCode == "" {
		operatorCode = stringField(payload, "operatorCode")
	}
	return payload, actionType, operatorCode, envelope, nil
}

func commonTripProjection(entry map[string]any) map[string]any {
	stageFields := map[string]struct{}{
		"tripStageCode": {}, "fromStation": {}, "toStation": {}, "stationPoint": {}, "stageFare": {},
	}
	common := make(map[string]any, len(entry))
	for key, value := range entry {
		if _, isStageField := stageFields[key]; isStageField {
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
	stage := make(map[string]any, len(entry))
	for key, value := range entry {
		if key == "bus" {
			if bus, ok := value.(map[string]any); ok {
				stage[key] = copyWithoutSeatLayoutList(bus)
				continue
			}
		}
		stage[key] = value
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

func suppliedSeatLayoutList(entry map[string]any) (any, bool) {
	if value, exists := entry["seatLayoutList"]; exists {
		return value, true
	}
	bus, ok := entry["bus"].(map[string]any)
	if !ok {
		return nil, false
	}
	value, exists := bus["seatLayoutList"]
	return value, exists
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func nestedStringField(object map[string]any, parent, key string) string {
	nested, _ := object[parent].(map[string]any)
	return stringField(nested, key)
}

func stageMetadataRecords(base domain.TripDetailsStageMetadata, entry map[string]any) []domain.TripDetailsStageMetadata {
	records := []domain.TripDetailsStageMetadata{base}
	seen := map[string]struct{}{base.FromStationCode + "\x00" + base.ToStationCode: {}}
	stations := stationCodesFromValue(entry)
	for fromIndex, fromCode := range stations {
		for _, toCode := range stations[fromIndex+1:] {
			key := fromCode + "\x00" + toCode
			if fromCode == "" || toCode == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			record := base
			record.FromStationCode = fromCode
			record.ToStationCode = toCode
			records = append(records, record)
		}
	}
	return records
}
