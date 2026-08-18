package master

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"orbitplusmaster/internal/domain"
)

var ErrTripDetailsNotFound = errors.New("trip details not found")

type RouteLookup struct {
	OperatorCode string
	TripCode     string
	FromCode     string
	ToCode       string
	TravelDate   string
}

type TripDetailsContentReader interface {
	GetJSON(context.Context, string) ([]byte, bool, error)
}

type TripDetailsStageReader interface {
	FindStagesByRoute(context.Context, string, string, string, string) ([]domain.TripDetailsStageMetadata, error)
	FindStagesByTripRoute(context.Context, string, string, string, string, string) ([]domain.TripDetailsStageMetadata, error)
}

type TripDetailsReadService struct {
	content  TripDetailsContentReader
	metadata TripDetailsStageReader
	logger   *log.Logger
}

func NewTripDetailsReadService(content TripDetailsContentReader, metadata TripDetailsStageReader, logger *log.Logger) *TripDetailsReadService {
	return &TripDetailsReadService{content: content, metadata: metadata, logger: logger}
}

func ValidSearchLookup(lookup RouteLookup) bool {
	return validLookupComponents(lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
}

func ValidBusMapLookup(lookup RouteLookup) bool {
	return validLookupComponents(lookup.OperatorCode, lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
}

func validLookupComponents(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.Contains(value, ":") {
			return false
		}
	}
	return true
}

// Search reconstructs every selected persisted stage for a route.
func (service *TripDetailsReadService) Search(ctx context.Context, lookup RouteLookup) ([]json.RawMessage, error) {
	service.logLookup("search", lookup)
	candidates, err := service.metadata.FindStagesByRoute(ctx, lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	if err != nil {
		service.logFailure("search metadata lookup")
		return nil, err
	}
	service.logCount("search metadata candidates", len(candidates))
	if len(candidates) == 0 {
		service.logNotFound("search", "metadata_empty")
		return nil, ErrTripDetailsNotFound
	}
	resolved, err := service.resolveStages(ctx, lookup, candidates)
	if err != nil {
		service.logFailure("search stage resolution")
		return nil, err
	}
	service.logCount("search resolved stages", len(resolved))
	if len(resolved) == 0 {
		service.logNotFound("search", "no_stage_matches_route")
		return nil, ErrTripDetailsNotFound
	}

	results := make([]json.RawMessage, 0, len(resolved))
	for _, candidate := range resolved {
		service.logCandidate("search selected stage", candidate.Metadata)
		tripKey, err := BuildTripKey(candidate.Metadata.OperatorCode, candidate.Metadata.TripCode)
		if err != nil {
			service.logFailure("search trip key")
			return nil, err
		}
		trip, found, err := service.content.GetJSON(ctx, tripKey)
		if err != nil {
			service.logFailure("search trip content")
			return nil, err
		}
		if !found {
			service.logCandidate("search trip missing", candidate.Metadata)
			continue
		}
		service.logCandidate("search trip found", candidate.Metadata)
		stage, err := withoutSeatLayoutList(candidate.Stage)
		if err != nil {
			service.logFailure("search stage sanitization")
			return nil, err
		}
		entry, err := mergeJSONObjects(trip, stage)
		if err != nil {
			service.logFailure("search reconstruction")
			return nil, err
		}
		results = append(results, entry)
	}
	if len(results) == 0 {
		service.logNotFound("search", "selected_trip_content_missing")
		return nil, ErrTripDetailsNotFound
	}
	service.logCount("search response entries", len(results))
	return results, nil
}

// BusMap returns the complete persisted BUSMAP contract entry for the first
// resolver-selected stage. Legacy layout-only documents retain their previous
// canonical reconstruction behavior.
func (service *TripDetailsReadService) BusMap(ctx context.Context, lookup RouteLookup) (json.RawMessage, error) {
	service.logLookup("busmap", lookup)
	candidates, err := service.metadata.FindStagesByTripRoute(ctx, lookup.OperatorCode, lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	if err != nil {
		service.logFailure("busmap metadata lookup")
		return nil, err
	}
	service.logCount("busmap metadata candidates", len(candidates))
	if len(candidates) == 0 {
		service.logNotFound("busmap", "metadata_empty")
		return nil, ErrTripDetailsNotFound
	}
	resolved, err := service.resolveStages(ctx, lookup, candidates)
	if err != nil {
		service.logFailure("busmap stage resolution")
		return nil, err
	}
	service.logCount("busmap resolved stages", len(resolved))
	if len(resolved) == 0 {
		service.logNotFound("busmap", "no_stage_matches_route")
		return nil, ErrTripDetailsNotFound
	}
	candidate := resolved[0]
	service.logCandidate("busmap selected stage", candidate.Metadata)
	busMapKey, err := BuildBusMapKey(candidate.Metadata.OperatorCode, candidate.Metadata.TripCode, candidate.Metadata.TripStageCode)
	if err != nil {
		service.logFailure("busmap response key")
		return nil, err
	}
	busMap, found, err := service.content.GetJSON(ctx, busMapKey)
	if err != nil {
		service.logFailure("busmap response content")
		return nil, err
	}
	if !found {
		service.logNotFound("busmap", "selected_busmap_content_missing")
		return nil, ErrTripDetailsNotFound
	}
	service.logCandidate("busmap response found", candidate.Metadata)
	if isCompleteBusMapResponse(busMap) {
		service.logCandidate("busmap response built", candidate.Metadata)
		return busMap, nil
	}

	tripKey, err := BuildTripKey(candidate.Metadata.OperatorCode, candidate.Metadata.TripCode)
	if err != nil {
		service.logFailure("busmap legacy trip key")
		return nil, err
	}
	trip, found, err := service.content.GetJSON(ctx, tripKey)
	if err != nil {
		service.logFailure("busmap legacy trip content")
		return nil, err
	}
	if !found {
		service.logNotFound("busmap", "selected_trip_content_missing")
		return nil, ErrTripDetailsNotFound
	}
	entry, err := mergeTripStageBusMap(trip, candidate.Stage, busMap)
	if err != nil {
		service.logFailure("busmap legacy reconstruction")
		return nil, err
	}
	service.logCandidate("busmap legacy response built", candidate.Metadata)
	return entry, nil
}

// isCompleteBusMapResponse identifies new dedicated BUSMAP documents. Older
// layout-only documents continue through the legacy canonical merge path.
// Trip and Stage identifiers are the stable discriminator so optional BUSMAP
// fields may be absent or null without changing how the response is served.
func isCompleteBusMapResponse(value []byte) bool {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(value, &document); err != nil {
		return false
	}
	if _, exists := document["tripCode"]; !exists {
		return false
	}
	_, exists := document["tripStageCode"]
	return exists
}

func (service *TripDetailsReadService) logLookup(operation string, lookup RouteLookup) {
	if service.logger != nil {
		service.logger.Printf("TripDetails %s lookup operatorCode=%q tripCode=%q fromCode=%q toCode=%q travelDate=%q", operation, lookup.OperatorCode, lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	}
}

func (service *TripDetailsReadService) logCandidate(operation string, metadata domain.TripDetailsStageMetadata) {
	if service.logger != nil {
		service.logger.Printf("TripDetails %s operatorCode=%q tripCode=%q tripStageCode=%q fromCode=%q toCode=%q travelDate=%q", operation, metadata.OperatorCode, metadata.TripCode, metadata.TripStageCode, metadata.FromStationCode, metadata.ToStationCode, metadata.TravelDate)
	}
}

func (service *TripDetailsReadService) logCount(operation string, count int) {
	if service.logger != nil {
		service.logger.Printf("TripDetails %s count=%d", operation, count)
	}
}

func (service *TripDetailsReadService) logNotFound(operation, reason string) {
	if service.logger != nil {
		service.logger.Printf("TripDetails %s outcome=not_found reason=%s", operation, reason)
	}
}

func (service *TripDetailsReadService) logFailure(operation string) {
	if service.logger != nil {
		service.logger.Printf("TripDetails read failed: %s", operation)
	}
}

type resolvedStage struct {
	Metadata domain.TripDetailsStageMetadata
	Stage    []byte
}

func (service *TripDetailsReadService) resolveStages(ctx context.Context, lookup RouteLookup, candidates []domain.TripDetailsStageMetadata) ([]resolvedStage, error) {
	resolved := make([]resolvedStage, 0, len(candidates))
	for _, candidate := range candidates {
		service.logCandidate("stage resolver candidate", candidate)
		stageKey, err := BuildStageKey(candidate.OperatorCode, candidate.TripCode, candidate.TripStageCode)
		if err != nil {
			service.logFailure("stage resolver key")
			return nil, err
		}
		stage, found, err := service.content.GetJSON(ctx, stageKey)
		if err != nil {
			service.logCandidate("stage resolver content_error", candidate)
			return nil, err
		}
		if !found {
			service.logCandidate("stage resolver content_missing", candidate)
			continue
		}
		if !stageContainsRoute(stage, lookup.FromCode, lookup.ToCode) {
			service.logCandidate("stage resolver route_not_matched", candidate)
			continue
		}
		service.logCandidate("stage resolver selected", candidate)
		resolved = append(resolved, resolvedStage{Metadata: candidate, Stage: stage})
	}
	return resolved, nil
}

func mergeJSONObjects(values ...[]byte) (json.RawMessage, error) {
	merged := map[string]json.RawMessage{}
	for _, value := range values {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(value, &object); err != nil {
			return nil, err
		}
		if object == nil {
			return nil, errors.New("stored content is not a JSON object")
		}
		for key, rawValue := range object {
			merged[key] = rawValue
		}
	}
	return json.Marshal(merged)
}

func mergeTripStageBusMap(trip, stage, busMap []byte) (json.RawMessage, error) {
	merged, err := mergeJSONObjects(trip, stage)
	if err != nil {
		return nil, err
	}
	var entry map[string]json.RawMessage
	var layout map[string]json.RawMessage
	if err := json.Unmarshal(merged, &entry); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(busMap, &layout); err != nil {
		return nil, err
	}
	seatLayoutList, exists := layout["seatLayoutList"]
	if !exists {
		return nil, errors.New("stored busmap has no seat layout")
	}
	busRaw, exists := entry["bus"]
	if !exists {
		return nil, errors.New("stored stage has no bus")
	}
	var bus map[string]json.RawMessage
	if err := json.Unmarshal(busRaw, &bus); err != nil || bus == nil {
		return nil, errors.New("stored bus is not a JSON object")
	}
	bus["seatLayoutList"] = seatLayoutList
	if totalSeatCount, supplied := layout["totalSeatCount"]; supplied && !isJSONNull(totalSeatCount) {
		bus["totalSeatCount"] = totalSeatCount
	}
	entry["bus"], err = json.Marshal(bus)
	if err != nil {
		return nil, err
	}
	if err := mergeBusMapAdditionalAttributes(entry, layout); err != nil {
		return nil, err
	}
	return json.Marshal(entry)
}

// mergeBusMapAdditionalAttributes preserves canonical attributes and adds
// BUSMAP attributes. A canonical key may be replaced only when explicitly
// classified as BUSMAP-owned.
func mergeBusMapAdditionalAttributes(entry, busMap map[string]json.RawMessage) error {
	busMapRaw, supplied := busMap["additionalAttributes"]
	if !supplied {
		return nil
	}
	var busMapAttributes map[string]json.RawMessage
	if err := json.Unmarshal(busMapRaw, &busMapAttributes); err != nil {
		return errors.New("stored busmap additionalAttributes is not a JSON object")
	}
	if busMapAttributes == nil {
		return nil
	}

	canonicalAttributes := map[string]json.RawMessage{}
	if canonicalRaw, exists := entry["additionalAttributes"]; exists {
		if err := json.Unmarshal(canonicalRaw, &canonicalAttributes); err != nil {
			return errors.New("stored canonical additionalAttributes is not a JSON object")
		}
		if canonicalAttributes == nil {
			canonicalAttributes = map[string]json.RawMessage{}
		}
	}
	for key, value := range busMapAttributes {
		if isJSONNull(value) {
			continue
		}
		if _, exists := canonicalAttributes[key]; !exists || isBusMapOwnedAdditionalAttribute(key) {
			canonicalAttributes[key] = value
		}
	}
	merged, err := json.Marshal(canonicalAttributes)
	if err != nil {
		return err
	}
	entry["additionalAttributes"] = merged
	return nil
}

func isBusMapOwnedAdditionalAttribute(key string) bool {
	return key == "stationPointSeatSelectionRequired"
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// withoutSeatLayoutList removes only the BUSMAP-owned layout from a legacy
// Stage document while retaining all other raw Stage fields.
func withoutSeatLayoutList(stage []byte) (json.RawMessage, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(stage, &entry); err != nil {
		return nil, err
	}
	busRaw, exists := entry["bus"]
	if !exists {
		return stage, nil
	}
	var bus map[string]json.RawMessage
	if err := json.Unmarshal(busRaw, &bus); err != nil || bus == nil {
		return nil, errors.New("stored stage bus is not a JSON object")
	}
	if _, exists := bus["seatLayoutList"]; !exists {
		return stage, nil
	}
	delete(bus, "seatLayoutList")
	var err error
	entry["bus"], err = json.Marshal(bus)
	if err != nil {
		return nil, err
	}
	return json.Marshal(entry)
}
