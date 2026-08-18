package master

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"

	"orbitplusmaster/internal/domain"
)

func TestSearchSelectsIntermediateRouteAndPreservesStoredFields(t *testing.T) {
	lookup := RouteLookup{OperatorCode: "bits", FromCode: "TRI", ToCode: "CHE", TravelDate: "2026-08-20"}
	candidate := stageMetadata("bits", "TRIP", "STAGE", "MAD", "CHE", "2026-08-20")
	cache := newReadCache(map[string]string{
		"stage:bits:TRIP:STAGE": `{"tripStageCode":"STAGE","stationPoint":[{"station":{"code":"MAD"}},{"station":{"code":"TRI"}},{"station":{"code":"CHE"}}],"fromStation":{"code":"MAD"},"toStation":{"code":"CHE"},"stageFare":[{"fare":2999,"availableSeatCount":7}],"bus":{"code":"BUS","seatLayoutList":[{"code":"LEGACY"}]},"unknown":{"nested":[true,null]}}`,
		"trip:bits:TRIP":        `{"tripCode":"TRIP","travelDate":"2026-08-20","bus":{"code":"BUS"},"operator":{"code":"bits"}}`,
	})
	metadata := &readMetadata{byRoute: []domain.TripDetailsStageMetadata{candidate}}
	service := NewTripDetailsReadService(cache, metadata, log.Default())

	results, err := service.Search(context.Background(), lookup)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d entries, want 1", len(results))
	}
	assertJSONContains(t, results[0], `{"tripCode":"TRIP","tripStageCode":"STAGE","stageFare":[{"fare":2999,"availableSeatCount":7}],"unknown":{"nested":[true,null]}}`)
	if containsJSONField(t, results[0], "seatLayoutList") {
		t.Fatal("Search response unexpectedly contains seatLayoutList")
	}
	if metadata.routeCalls != 1 || metadata.tripRouteCalls != 0 {
		t.Fatalf("metadata calls = route:%d tripRoute:%d", metadata.routeCalls, metadata.tripRouteCalls)
	}
}

func TestBusMapReturnsSelectedStageAndRawSeatLayout(t *testing.T) {
	lookup := RouteLookup{OperatorCode: "bits", TripCode: "TRIP", FromCode: "TRI", ToCode: "CHE", TravelDate: "2026-08-20"}
	candidate := stageMetadata("bits", "TRIP", "STAGE", "MAD", "CHE", "2026-08-20")
	cache := newReadCache(map[string]string{
		"stage:bits:TRIP:STAGE":  `{"tripStageCode":"STAGE","stationPoint":[{"station":{"code":"MAD"}},{"station":{"code":"TRI"}},{"station":{"code":"CHE"}}],"fromStation":{"code":"MAD"},"toStation":{"code":"CHE"},"bus":{"code":"BUS","seatLayoutList":[{"code":"OLD"}]}}`,
		"trip:bits:TRIP":         `{"tripCode":"TRIP","travelDate":"2026-08-20","schedule":{"code":"S1"},"cancellationTerm":{"value":"kept"}}`,
		"busmap:bits:TRIP:STAGE": `{"seatLayoutList":[{"code":"A1","fare":2999,"unknown":{"nested":true}},{"code":"A2","value":null}]}`,
	})
	metadata := &readMetadata{byTripRoute: []domain.TripDetailsStageMetadata{candidate}}
	service := NewTripDetailsReadService(cache, metadata, log.Default())

	result, err := service.BusMap(context.Background(), lookup)
	if err != nil {
		t.Fatalf("BusMap() error = %v", err)
	}
	assertJSONContains(t, result, `{"tripCode":"TRIP","tripStageCode":"STAGE","schedule":{"code":"S1"},"cancellationTerm":{"value":"kept"},"bus":{"code":"BUS","seatLayoutList":[{"code":"A1","fare":2999,"unknown":{"nested":true}},{"code":"A2","value":null}]}}`)
}

func TestMergeTripStageBusMapAppliesOnlyExplicitBusMapFields(t *testing.T) {
	trip := []byte(`{"tripCode":"TRIP","travelTime":"1 : 35","bus":{"code":"SEARCH-BUS","busType":"2+1 A/C Sleeper ","categoryCode":"SEARCH","displayName":"Search display","name":"Search name","totalSeatCount":30},"toStation":{"stationPoint":[{"name":"Central Bus Stand"},{"name":"Chathiram Bus Stand RKT"},{"name":"Srirangam Bye pass Road"},{"name":"Samayapuram Bus Sand1"}]},"additionalAttributes":{"someSearchAttribute":true,"stationPointSeatSelectionRequired":true}}`)
	stage := []byte(`{"tripStageCode":"STAGE","stageFare":[{"seatName":"Lower Sleeper","availableSeatCount":17},{"seatName":"Upper Sleeper","availableSeatCount":17}]}`)
	busMap := []byte(`{"seatLayoutList":[{"code":"A1","seatStatus":{"code":"AL"}},{"code":"PTY","seatStatus":{"code":"BL"}}],"totalSeatCount":36,"additionalAttributes":{"stationPointSeatSelectionRequired":false}}`)

	result, err := mergeTripStageBusMap(trip, stage, busMap)
	if err != nil {
		t.Fatalf("mergeTripStageBusMap() error = %v", err)
	}
	assertJSONContains(t, result, `{"tripCode":"TRIP","tripStageCode":"STAGE","travelTime":"1 : 35","bus":{"code":"SEARCH-BUS","busType":"2+1 A/C Sleeper ","categoryCode":"SEARCH","displayName":"Search display","name":"Search name","totalSeatCount":36,"seatLayoutList":[{"code":"A1","seatStatus":{"code":"AL"}},{"code":"PTY","seatStatus":{"code":"BL"}}]},"additionalAttributes":{"someSearchAttribute":true,"stationPointSeatSelectionRequired":false},"toStation":{"stationPoint":[{"name":"Central Bus Stand"},{"name":"Chathiram Bus Stand RKT"},{"name":"Srirangam Bye pass Road"},{"name":"Samayapuram Bus Sand1"}]},"stageFare":[{"seatName":"Lower Sleeper","availableSeatCount":17},{"seatName":"Upper Sleeper","availableSeatCount":17}]}`)
}

func TestMergeTripStageBusMapDoesNotLetNullEraseCanonicalFields(t *testing.T) {
	trip := []byte(`{"travelTime":"1 : 35","bus":{"busType":"2+1 A/C Sleeper ","totalSeatCount":30},"additionalAttributes":{"someSearchAttribute":true,"stationPointSeatSelectionRequired":true}}`)
	busMap := []byte(`{"seatLayoutList":[{"code":"A1"}],"totalSeatCount":null,"additionalAttributes":{"stationPointSeatSelectionRequired":null}}`)

	result, err := mergeTripStageBusMap(trip, []byte(`{}`), busMap)
	if err != nil {
		t.Fatalf("mergeTripStageBusMap() error = %v", err)
	}
	assertJSONContains(t, result, `{"travelTime":"1 : 35","bus":{"busType":"2+1 A/C Sleeper ","totalSeatCount":30,"seatLayoutList":[{"code":"A1"}]},"additionalAttributes":{"someSearchAttribute":true,"stationPointSeatSelectionRequired":true}}`)
}

func TestBusMapReturnsCompleteBitsResponseWithOptionalFieldsMissing(t *testing.T) {
	lookup := RouteLookup{OperatorCode: "bits", TripCode: "TRIP", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"}
	candidate := stageMetadata("bits", "TRIP", "STAGE", "MAD", "CHE", "2026-08-20")
	busMap := `{"tripCode":"TRIP","tripStageCode":"STAGE","travelTime":null,"bus":null,"additionalAttributes":null}`
	cache := newReadCache(map[string]string{
		"stage:bits:TRIP:STAGE":  `{"tripStageCode":"STAGE","fromStation":{"code":"MAD"},"toStation":{"code":"CHE"}}`,
		"busmap:bits:TRIP:STAGE": busMap,
	})
	service := NewTripDetailsReadService(cache, &readMetadata{byTripRoute: []domain.TripDetailsStageMetadata{candidate}}, log.Default())

	result, err := service.BusMap(context.Background(), lookup)
	if err != nil {
		t.Fatalf("BusMap() error = %v", err)
	}
	assertJSONEqual(t, result, busMap)
}

func TestBusMapReturnsNotFoundWhenDedicatedResponseIsMissing(t *testing.T) {
	lookup := RouteLookup{OperatorCode: "bits", TripCode: "TRIP", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"}
	candidate := stageMetadata("bits", "TRIP", "STAGE", "MAD", "CHE", "2026-08-20")
	cache := newReadCache(map[string]string{
		"stage:bits:TRIP:STAGE": `{"tripStageCode":"STAGE","fromStation":{"code":"MAD"},"toStation":{"code":"CHE"}}`,
	})
	service := NewTripDetailsReadService(cache, &readMetadata{byTripRoute: []domain.TripDetailsStageMetadata{candidate}}, log.Default())

	_, err := service.BusMap(context.Background(), lookup)
	if !errors.Is(err, ErrTripDetailsNotFound) {
		t.Fatalf("BusMap() error = %v, want not found", err)
	}
}

func TestSearchReturnsNotFoundForMissingStageOrTrip(t *testing.T) {
	lookup := RouteLookup{OperatorCode: "bits", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"}
	candidate := stageMetadata("bits", "TRIP", "STAGE", "MAD", "CHE", "2026-08-20")
	stage := `{"tripStageCode":"STAGE","fromStation":{"code":"MAD"},"toStation":{"code":"CHE"}}`
	tests := []struct {
		name  string
		cache map[string]string
	}{
		{"stage not found", map[string]string{}},
		{"trip not found", map[string]string{"stage:bits:TRIP:STAGE": stage}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewTripDetailsReadService(newReadCache(test.cache), &readMetadata{byRoute: []domain.TripDetailsStageMetadata{candidate}}, log.Default())
			_, err := service.Search(context.Background(), lookup)
			if !errors.Is(err, ErrTripDetailsNotFound) {
				t.Fatalf("Search() error = %v, want not found", err)
			}
		})
	}
}

func TestReadLookupsRejectInvalidParameters(t *testing.T) {
	if ValidSearchLookup(RouteLookup{OperatorCode: "bits", FromCode: "", ToCode: "CHE", TravelDate: "2026-08-20"}) {
		t.Fatal("empty Search origin was accepted")
	}
	if ValidBusMapLookup(RouteLookup{OperatorCode: "bits", TripCode: "TRIP:1", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"}) {
		t.Fatal("invalid Busmap trip code was accepted")
	}
}

func stageMetadata(operator, trip, stage, from, to, date string) domain.TripDetailsStageMetadata {
	return domain.TripDetailsStageMetadata{OperatorCode: operator, TripCode: trip, TripStageCode: stage, FromStationCode: from, ToStationCode: to, TravelDate: date}
}

type readCache struct{ values map[string][]byte }

func newReadCache(values map[string]string) *readCache {
	cache := &readCache{values: map[string][]byte{}}
	for key, value := range values {
		cache.values[key] = []byte(value)
	}
	return cache
}

func (cache *readCache) GetJSON(_ context.Context, key string) ([]byte, bool, error) {
	value, found := cache.values[key]
	return value, found, nil
}

type readMetadata struct {
	byRoute        []domain.TripDetailsStageMetadata
	byTripRoute    []domain.TripDetailsStageMetadata
	routeCalls     int
	tripRouteCalls int
}

func (metadata *readMetadata) FindStagesByRoute(_ context.Context, _, _, _, _ string) ([]domain.TripDetailsStageMetadata, error) {
	metadata.routeCalls++
	return metadata.byRoute, nil
}

func (metadata *readMetadata) FindStagesByTripRoute(_ context.Context, _, _, _, _, _ string) ([]domain.TripDetailsStageMetadata, error) {
	metadata.tripRouteCalls++
	return metadata.byTripRoute, nil
}

func assertJSONContains(t *testing.T, actual []byte, expected string) {
	t.Helper()
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("decode actual: %v", err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("decode expected: %v", err)
	}
	if !containsJSON(expectedValue, actualValue) {
		t.Fatalf("actual = %s, want at least %s", actual, expected)
	}
}

func containsJSONField(t *testing.T, raw []byte, field string) bool {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return hasJSONField(value, field)
}

func hasJSONField(value any, field string) bool {
	switch value := value.(type) {
	case map[string]any:
		if _, exists := value[field]; exists {
			return true
		}
		for _, nested := range value {
			if hasJSONField(nested, field) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if hasJSONField(nested, field) {
				return true
			}
		}
	}
	return false
}

func TestSearchLogsLookupAndNotFoundReasonWithoutCredentials(t *testing.T) {
	var output bytes.Buffer
	lookup := RouteLookup{OperatorCode: "bits", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"}
	service := NewTripDetailsReadService(newReadCache(nil), &readMetadata{}, log.New(&output, "", 0))

	_, err := service.Search(context.Background(), lookup)
	if !errors.Is(err, ErrTripDetailsNotFound) {
		t.Fatalf("Search() error = %v, want not found", err)
	}
	logs := output.String()
	for _, expected := range []string{
		`TripDetails search lookup operatorCode="bits" tripCode="" fromCode="MAD" toCode="CHE" travelDate="2026-08-20"`,
		"TripDetails search metadata candidates count=0",
		"TripDetails search outcome=not_found reason=metadata_empty",
	} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("logs do not contain %q: %s", expected, logs)
		}
	}
	for _, secret := range []string{"demo-user", "demo-token", "payload-marker"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs expose sensitive value %q: %s", secret, logs)
		}
	}
}
