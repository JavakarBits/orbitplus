package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

func TestReadRoutesServePersistedSearchAndBusMap(t *testing.T) {
	metadata := &readTestMetadata{stage: domain.TripDetailsStageMetadata{OperatorCode: "bits", TripCode: "TRIP", TripStageCode: "STAGE", FromStationCode: "MAD", ToStationCode: "CHE", TravelDate: "2026-08-20"}}
	content := &readTestContent{values: map[string][]byte{
		"trip:bits:TRIP":         []byte(`{"tripCode":"TRIP","travelDate":"2026-08-20"}`),
		"stage:bits:TRIP:STAGE":  []byte(`{"tripStageCode":"STAGE","fromStation":{"code":"MAD"},"toStation":{"code":"CHE"},"bus":{"code":"BUS"}}`),
		"busmap:bits:TRIP:STAGE": []byte(`{"seatLayoutList":[{"code":"A1"}]}`),
	}}
	readService := master.NewTripDetailsReadService(content, metadata, nil)
	router := NewRouter(master.NewTripDetailsService(), readService)

	for _, path := range []string{
		"/busservices/api/3.0/json/bits/user/secret/search/MAD/CHE/2026-08-20",
		"/busservices/api/3.0/json/bits/user/secret/busmap/TRIP/MAD/CHE/2026-08-20",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body = %s", path, recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%s Content-Type = %q", path, recorder.Header().Get("Content-Type"))
		}
		assertReadResponseDatetime(t, recorder)
	}
}

func TestReadRouteRejectsMalformedPath(t *testing.T) {
	router := NewRouter(master.NewTripDetailsService(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/busservices/api/3.0/json/bits/user/token/search/MAD", nil))
	assertJSONStatus(t, recorder, http.StatusBadRequest, 0, "Invalid request")
}

type readTestContent struct{ values map[string][]byte }

func (content *readTestContent) GetJSON(_ context.Context, key string) ([]byte, bool, error) {
	value, found := content.values[key]
	return value, found, nil
}

type readTestMetadata struct {
	stage domain.TripDetailsStageMetadata
}

func (metadata *readTestMetadata) FindStagesByRoute(_ context.Context, _, _, _, _ string) ([]domain.TripDetailsStageMetadata, error) {
	return []domain.TripDetailsStageMetadata{metadata.stage}, nil
}

func (metadata *readTestMetadata) FindStagesByTripRoute(_ context.Context, _, _, _, _, _ string) ([]domain.TripDetailsStageMetadata, error) {
	return []domain.TripDetailsStageMetadata{metadata.stage}, nil
}

func assertReadResponseDatetime(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var response struct {
		Status   int    `json:"status"`
		Datetime string `json:"datetime"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Status != 1 {
		t.Fatalf("status = %d, want 1", response.Status)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", response.Datetime); err != nil {
		t.Fatalf("datetime = %q, want YYYY-MM-DD HH:MM:SS: %v", response.Datetime, err)
	}
}
