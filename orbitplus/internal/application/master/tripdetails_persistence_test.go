package master

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"strings"
	"testing"

	"orbitplusmaster/internal/domain"
)

func TestTripDetailsPersistenceStoresIndependentStagesAndMetadata(t *testing.T) {
	cache := newMemoryCache()
	metadata := &memoryMetadata{}
	persistence := NewTripDetailsPersistence(cache, metadata)
	value := mustJSON(t, `{
		"actionType":"SEARCH","operatorCode":"bits","orbitResponse":{"data":[
			{"tripCode":"2N38731S260820D","tripStageCode":"2N38731S260820D2T1","travelDate":"2026-08-20","displayName":"NA","fromStation":{"code":"STF17D52"},"toStation":{"code":"STF17D51"},"stageFare":[{"availableSeatCount":17}],"unknown":{"nested":[true,"kept"]}},
			{"tripCode":"2N38731S260820D","tripStageCode":"2N38731S260820D2T23","travelDate":"2026-08-20","fromStation":{"code":"STF17D52"},"toStation":{"code":"STF1GI77"},"stageFare":[{"availableSeatCount":9}]}
		]}}
	`)
	if err := persistence.Persist(context.Background(), value); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	assertCacheJSON(t, cache, "trip:bits:2N38731S260820D", `{"tripCode":"2N38731S260820D","travelDate":"2026-08-20"}`)
	assertCacheJSON(t, cache, "stage:bits:2N38731S260820D:2N38731S260820D2T1", `{"tripStageCode":"2N38731S260820D2T1","fromStation":{"code":"STF17D52"},"toStation":{"code":"STF17D51"},"stageFare":[{"availableSeatCount":17}],"unknown":{"nested":[true,"kept"]}}`)
	assertCacheJSON(t, cache, "stage:bits:2N38731S260820D:2N38731S260820D2T23", `{"tripStageCode":"2N38731S260820D2T23","fromStation":{"code":"STF17D52"},"toStation":{"code":"STF1GI77"},"stageFare":[{"availableSeatCount":9}]}`)
	if len(metadata.values) != 2 || metadata.values[1].TripStageCode != "2N38731S260820D2T23" {
		t.Fatalf("metadata = %#v, want two independent stages", metadata.values)
	}
	if _, exists := cache.values["busmap:bits:2N38731S260820D:2N38731S260820D2T1"]; exists {
		t.Fatal("SEARCH created a BUSMAP cache entry")
	}
}

func TestTripDetailsPersistenceStoresOnlyBusMapForSupportedEnvelopes(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "worker root",
			value: `{"actionType":"busmap","operatorCode":"bits","orbitResponse":{"tripCode":"ABC","tripStageCode":"ABC-T1","bus":{"seatLayoutList":[{"code":"A1","seatFare":2999,"unknown":{"nested":true}}]}}}`,
		},
		{
			name:  "worker data object",
			value: `{"actionType":"busmap","operatorCode":"bits","orbitResponse":{"status":1,"data":{"tripCode":"ABC","tripStageCode":"ABC-T1","bus":{"seatLayoutList":[{"code":"A1","seatFare":2999,"unknown":{"nested":true}}]}}}}`,
		},
		{
			name:  "direct inferred action",
			value: `{"status":1,"data":{"tripCode":"ABC","tripStageCode":"ABC-T1","operator":{"code":"bits"},"bus":{"seatLayoutList":[{"code":"A1","seatFare":2999,"unknown":{"nested":true}}]}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := newMemoryCache()
			metadata := &memoryMetadata{}
			if err := NewTripDetailsPersistence(cache, metadata).Persist(context.Background(), mustJSON(t, test.value)); err != nil {
				t.Fatalf("Persist() error = %v", err)
			}
			assertCacheJSON(t, cache, "busmap:bits:ABC:ABC-T1", `{"tripCode":"ABC","tripStageCode":"ABC-T1","bus":{"seatLayoutList":[{"code":"A1","seatFare":2999,"unknown":{"nested":true}}]}}`)
			if len(cache.values) != 1 {
				t.Fatalf("cache values = %v, want only BUSMAP", cache.values)
			}
			if len(metadata.values) != 0 {
				t.Fatalf("metadata = %#v, want no BUSMAP metadata writes", metadata.values)
			}
		})
	}
}

func TestTripDetailsPersistenceStoresCompleteBusMapResponse(t *testing.T) {
	cache := newMemoryCache()
	response := `{"tripCode":"TRIP","tripStageCode":"STAGE","travelTime":"1:35","bus":{"busType":"2+1 A/C Sleeper","totalSeatCount":36,"seatLayoutList":[{"code":"A1"}]},"additionalAttributes":{"stationPointSeatSelectionRequired":false}}`
	value := mustJSON(t, `{"actionType":"BUSMAP","operatorCode":"bits","orbitResponse":`+response+`}`)
	if err := NewTripDetailsPersistence(cache, &memoryMetadata{}).Persist(context.Background(), value); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	assertCacheJSONEqual(t, cache, "busmap:bits:TRIP:STAGE", response)
}

func TestTripDetailsPersistenceStoresBusMapWithMissingOptionalFields(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "worker action",
			value: `{"actionType":"BUSMAP","operatorCode":"bits","orbitResponse":{"tripCode":"TRIP","tripStageCode":"STAGE","bus":null,"additionalAttributes":null}}`,
		},
		{
			name:  "direct single data object",
			value: `{"status":1,"data":{"tripCode":"TRIP","tripStageCode":"STAGE","operator":{"code":"bits"},"bus":null,"additionalAttributes":null}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := newMemoryCache()
			if err := NewTripDetailsPersistence(cache, &memoryMetadata{}).Persist(context.Background(), mustJSON(t, test.value)); err != nil {
				t.Fatalf("Persist() error = %v", err)
			}
			assertCacheJSON(t, cache, "busmap:bits:TRIP:STAGE", `{"tripCode":"TRIP","tripStageCode":"STAGE","bus":null,"additionalAttributes":null}`)
		})
	}
}

func TestTripDetailsPersistencePreservesSearchAndBusMapContractParity(t *testing.T) {
	cache := newMemoryCache()
	metadata := &memoryMetadata{}
	persistence := NewTripDetailsPersistence(cache, metadata)
	searchEntry := `{
		"tripCode":"TRIP","tripStageCode":"STAGE","travelDate":"2026-08-20","tripDate":"2026-08-20","displayName":"Search display","travelTime":"1 : 35","closeTime":"2026-08-20 22:00:00",
		"bus":{"code":"BUS","busType":"2+1 A/C Sleeper ","categoryCode":"SEARCH","displayName":"Search bus","name":"Search bus","totalSeatCount":30},
		"fromStation":{"code":"MAD","stationPoint":[{"code":"FROM-1","name":"First"}]},
		"toStation":{"code":"CHE","stationPoint":[{"code":"TO-1","name":"Central"},{"code":"TO-2","name":"Middle"},{"code":"TO-3","name":"Last"}]},
		"schedule":{"code":"SEARCH-SCHEDULE"},"operator":{"code":"bits"},"cancellationTerm":{"code":"SEARCH-CANCEL"},"tripStatus":{"code":"OPEN"},
		"stageFare":[{"seatName":"Lower Sleeper","availableSeatCount":17}],"amenities":[{"code":"SEARCH-AMENITY"}],"activities":[],"viaStations":[{"code":"MAD"},{"code":"CHE"}],"additionalAttributes":{"searchOnly":true}
	}`
	search := mustJSON(t, `{"actionType":"SEARCH","operatorCode":"bits","orbitResponse":{"data":[`+searchEntry+`]}}`)
	if err := persistence.Persist(context.Background(), search); err != nil {
		t.Fatalf("persist SEARCH: %v", err)
	}
	tripBefore := append([]byte(nil), cache.values["trip:bits:TRIP"]...)
	stageBefore := append([]byte(nil), cache.values["stage:bits:TRIP:STAGE"]...)
	metadataCount := len(metadata.values)

	busMapEntry := `{
		"tripCode":"TRIP","tripStageCode":"STAGE","travelDate":"2026-08-20","tripDate":"2026-08-20","displayName":"BUSMAP display","travelTime":"1:35","closeTime":"2026-08-20 22:15:00",
		"bus":{"code":"BUS","busType":"2+1 A/C Sleeper","categoryCode":"BUSMAP","displayName":"BUSMAP bus","name":"BUSMAP bus","totalSeatCount":36,"seatLayoutList":[{"code":"A1","seatStatus":{"code":"AL"}},{"code":"PTY","seatStatus":{"code":"BL"}}]},
		"fromStation":{"code":"MAD","stationPoint":[{"code":"FROM-1","name":"First"}]},
		"toStation":{"code":"CHE","stationPoint":[{"code":"TO-3","name":"Last"},{"code":"TO-2","name":"Middle"},{"code":"TO-1","name":"Central"}]},
		"schedule":{"code":"BUSMAP-SCHEDULE"},"operator":{"code":"bits"},"cancellationTerm":{"code":"BUSMAP-CANCEL"},"tripStatus":{"code":"OPEN"},
		"stageFare":[{"seatName":"Lower Sleeper","availableSeatCount":17}],"amenities":[{"code":"BUSMAP-AMENITY"}],"activities":[{"code":"BUSMAP-ACTIVITY"}],"viaStations":[{"code":"CHE"},{"code":"MAD"}],"additionalAttributes":{"stationPointSeatSelectionRequired":false}
	}`
	busMap := mustJSON(t, `{"actionType":"BUSMAP","operatorCode":"bits","orbitResponse":`+busMapEntry+`}`)
	if err := persistence.Persist(context.Background(), busMap); err != nil {
		t.Fatalf("persist BUSMAP: %v", err)
	}

	if !bytes.Equal(tripBefore, cache.values["trip:bits:TRIP"]) || !bytes.Equal(stageBefore, cache.values["stage:bits:TRIP:STAGE"]) {
		t.Fatal("BUSMAP modified SEARCH-owned Trip or Stage content")
	}
	if len(metadata.values) != metadataCount {
		t.Fatalf("BUSMAP changed metadata count from %d to %d", metadataCount, len(metadata.values))
	}
	assertCacheJSONEqual(t, cache, "busmap:bits:TRIP:STAGE", busMapEntry)

	readMetadata := &readMetadata{byRoute: metadata.values, byTripRoute: metadata.values}
	readCache := newReadCache(map[string]string{
		"trip:bits:TRIP":         string(cache.values["trip:bits:TRIP"]),
		"stage:bits:TRIP:STAGE":  string(cache.values["stage:bits:TRIP:STAGE"]),
		"busmap:bits:TRIP:STAGE": string(cache.values["busmap:bits:TRIP:STAGE"]),
	})
	service := NewTripDetailsReadService(readCache, readMetadata, log.Default())
	searchResults, err := service.Search(context.Background(), RouteLookup{OperatorCode: "bits", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"})
	if err != nil || len(searchResults) != 1 {
		t.Fatalf("Search() = %s, %v", searchResults, err)
	}
	assertJSONEqual(t, searchResults[0], searchEntry)

	busMapResult, err := service.BusMap(context.Background(), RouteLookup{OperatorCode: "bits", TripCode: "TRIP", FromCode: "MAD", ToCode: "CHE", TravelDate: "2026-08-20"})
	if err != nil {
		t.Fatalf("BusMap() error = %v", err)
	}
	assertJSONEqual(t, busMapResult, busMapEntry)
}

func TestTripDetailsPersistenceRejectsMissingIdentifiersAndWriteFailures(t *testing.T) {
	valid := mustJSON(t, `{"actionType":"SEARCH","operatorCode":"bits","orbitResponse":{"data":[{"tripCode":"ABC","tripStageCode":"T1","travelDate":"2026-08-20","fromStation":{"code":"FROM"},"toStation":{"code":"TO"}}]}}`)
	missing := mustJSON(t, `{"actionType":"SEARCH","orbitResponse":{"data":[{}]}}`)
	if err := NewTripDetailsPersistence(newMemoryCache(), &memoryMetadata{}).Persist(context.Background(), missing); !errors.Is(err, errMissingPersistenceIdentifiers) {
		t.Fatalf("missing identifiers error = %v", err)
	}
	cacheFailure := newMemoryCache()
	cacheFailure.err = errors.New("Dragonfly unavailable")
	if err := NewTripDetailsPersistence(cacheFailure, &memoryMetadata{}).Persist(context.Background(), valid); err == nil {
		t.Fatal("cache failure was acknowledged")
	}
	metadataFailure := &memoryMetadata{err: errors.New("Cassandra unavailable")}
	if err := NewTripDetailsPersistence(newMemoryCache(), metadataFailure).Persist(context.Background(), valid); err == nil {
		t.Fatal("metadata failure was acknowledged")
	}
}

func TestTripDetailsPersistenceUsesDirectBitsEnvelopeAndLatestWrite(t *testing.T) {
	cache := newMemoryCache()
	metadata := &memoryMetadata{}
	persistence := NewTripDetailsPersistence(cache, metadata)
	first := mustJSON(t, `{"data":[{"operator":{"code":"bits"},"tripCode":"ABC","tripStageCode":"T1","travelDate":"2026-08-20","fromStation":{"code":"FROM"},"toStation":{"code":"TO"},"displayName":"first"}]}`)
	second := mustJSON(t, `{"data":[{"operator":{"code":"bits"},"tripCode":"ABC","tripStageCode":"T1","travelDate":"2026-08-20","fromStation":{"code":"FROM"},"toStation":{"code":"TO"},"displayName":"second"}]}`)
	if err := persistence.Persist(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := persistence.Persist(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	assertCacheJSON(t, cache, "stage:bits:ABC:T1", `{"displayName":"second","tripStageCode":"T1"}`)
	if len(metadata.values) != 2 || metadata.values[1].TripStageCode != "T1" {
		t.Fatalf("metadata values = %#v", metadata.values)
	}
}

func TestTripDetailsPersistenceLeavesSearchBusMapAsPhaseTwoTODO(t *testing.T) {
	cache := newMemoryCache()
	metadata := &memoryMetadata{}
	value := mustJSON(t, `{"actionType":"SEARCHBUSMAP","orbitResponse":{"data":[{}]}}`)
	if err := NewTripDetailsPersistence(cache, metadata).Persist(context.Background(), value); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if len(cache.values) != 0 || len(metadata.values) != 0 {
		t.Fatalf("SEARCHBUSMAP was persisted: cache=%v metadata=%v", cache.values, metadata.values)
	}
}

func TestTripDetailsCacheKeysValidateComponents(t *testing.T) {
	if key, err := BuildTripKey("bits", "ABC"); err != nil || key != "trip:bits:ABC" {
		t.Fatalf("BuildTripKey() = %q, %v", key, err)
	}
	if key, err := BuildStageKey("bits", "ABC", "T1"); err != nil || key != "stage:bits:ABC:T1" {
		t.Fatalf("BuildStageKey() = %q, %v", key, err)
	}
	if key, err := BuildBusMapKey("bits", "ABC", "T1"); err != nil || key != "busmap:bits:ABC:T1" {
		t.Fatalf("BuildBusMapKey() = %q, %v", key, err)
	}
	if _, err := BuildStageKey("bits", "", "T1"); !errors.Is(err, errMissingPersistenceIdentifiers) {
		t.Fatalf("missing key component error = %v", err)
	}
}

type memoryCache struct {
	values map[string][]byte
	err    error
}

func newMemoryCache() *memoryCache { return &memoryCache{values: map[string][]byte{}} }
func (cache *memoryCache) SetJSON(_ context.Context, key string, value []byte) error {
	if cache.err != nil {
		return cache.err
	}
	cache.values[key] = append([]byte(nil), value...)
	return nil
}

type memoryMetadata struct {
	values []domain.TripDetailsStageMetadata
	err    error
}

func (metadata *memoryMetadata) SaveStageMetadata(_ context.Context, value domain.TripDetailsStageMetadata) error {
	if metadata.err != nil {
		return metadata.err
	}
	metadata.values = append(metadata.values, value)
	return nil
}

func mustJSON(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}

func assertCacheJSON(t *testing.T, cache *memoryCache, key, expected string) {
	t.Helper()
	actual, exists := cache.values[key]
	if !exists {
		t.Fatalf("cache key %q was not written", key)
	}
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("decode cached JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if !containsJSON(expectedValue, actualValue) {
		t.Fatalf("cache value for %q = %#v, want at least %#v", key, actualValue, expectedValue)
	}
}

func assertCacheJSONEqual(t *testing.T, cache *memoryCache, key, expected string) {
	t.Helper()
	actual, exists := cache.values[key]
	if !exists {
		t.Fatalf("cache key %q was not written", key)
	}
	assertJSONEqual(t, actual, expected)
}

func assertJSONEqual(t *testing.T, actual []byte, expected string) {
	t.Helper()
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("decode actual JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if !reflect.DeepEqual(expectedValue, actualValue) {
		t.Fatalf("actual JSON = %s, want semantically equal to %s", actual, expected)
	}
}

func containsJSON(expected, actual any) bool {
	expectedObject, expectedIsObject := expected.(map[string]any)
	if expectedIsObject {
		actualObject, actualIsObject := actual.(map[string]any)
		if !actualIsObject {
			return false
		}
		for key, expectedValue := range expectedObject {
			actualValue, exists := actualObject[key]
			if !exists || !containsJSON(expectedValue, actualValue) {
				return false
			}
		}
		return true
	}
	expectedList, expectedIsList := expected.([]any)
	if expectedIsList {
		actualList, actualIsList := actual.([]any)
		if !actualIsList || len(expectedList) != len(actualList) {
			return false
		}
		for index := range expectedList {
			if !containsJSON(expectedList[index], actualList[index]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(expected, actual)
}

func TestTripDetailsPersistenceLogsWriteProgressWithoutPayload(t *testing.T) {
	cache := newMemoryCache()
	metadata := &memoryMetadata{}
	var output bytes.Buffer
	persistence := NewTripDetailsPersistenceWithLogger(cache, metadata, log.New(&output, "", 0))
	value := mustJSON(t, `{"actionType":"search","operatorCode":"bits","orbitResponse":{"data":[{"tripCode":"ABC","tripStageCode":"T1","travelDate":"2026-08-20","fromStation":{"code":"FROM"},"toStation":{"code":"TO"},"secret":"PAYLOAD_MARKER"}]}}`)
	if err := persistence.Persist(context.Background(), value); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	logs := output.String()
	for _, expected := range []string{
		`envelope=worker actionType="search" normalizedActionType="SEARCH" entries=1`,
		`trip="trip:bits:ABC" stage="stage:bits:ABC:T1"`,
		`cache=trip key="trip:bits:ABC"`,
		`cache=stage key="stage:bits:ABC:T1"`,
		`Cassandra metadata write succeeded entry=0 tripStageCode="T1"`,
	} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("logs do not contain %q: %s", expected, logs)
		}
	}
	if strings.Contains(logs, "PAYLOAD_MARKER") {
		t.Fatalf("logs expose payload content: %s", logs)
	}
}

func assertCacheFieldMissing(t *testing.T, cache *memoryCache, key, field string) {
	t.Helper()
	raw, exists := cache.values[key]
	if !exists {
		t.Fatalf("cache key %q was not written", key)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode cached JSON: %v", err)
	}
	var bus map[string]json.RawMessage
	if err := json.Unmarshal(document["bus"], &bus); err != nil {
		t.Fatalf("decode cached bus JSON: %v", err)
	}
	if _, exists := bus[field]; exists {
		t.Fatalf("cache key %q unexpectedly contains bus.%s", key, field)
	}
}
