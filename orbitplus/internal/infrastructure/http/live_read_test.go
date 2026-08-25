package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"orbitplusmaster/internal/application/master"
)

// recordingFetcher captures the lookup the verifier hands to the Bits adapter.
//
// The real adapter would need a reachable zone host, and the zone table names
// production endpoints, so the outbound call is stubbed here instead. What
// matters is which endpoint and credential the lookup carried by the time it
// left the application layer.
type recordingFetcher struct {
	lookup master.BitsLookup
	called bool
}

func (fetcher *recordingFetcher) FetchTripDetails(_ context.Context, lookup master.BitsLookup) (master.BitsResult, error) {
	fetcher.lookup = lookup
	fetcher.called = true
	return master.BitsResult{
		Data:     json.RawMessage(`[{"tripCode":"T1"}]`),
		DataKind: master.BitsDataKindArray,
	}, nil
}

const searchRoutePattern = "GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}"

func newLiveHandler(t *testing.T, fetcher master.BitsTripDetailsFetcher) *TripDetailsReadHandler {
	t.Helper()
	verifier, err := master.NewCacheFreshnessVerifier(fetcher, nil, nil, nil, 1, log.Default())
	if err != nil {
		t.Fatalf("NewCacheFreshnessVerifier: %v", err)
	}
	return NewTripDetailsReadHandler(nil, verifier)
}

func serveLiveRequest(t *testing.T, handler *TripDetailsReadHandler, requestPath string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(searchRoutePattern, handler.ServeSearch)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
	return recorder
}

// The zone names the endpoint and the path supplies the credential that reaches
// the Bits adapter.
func TestLiveReadUsesPathCredentialAndZone(t *testing.T) {
	fetcher := &recordingFetcher{}
	handler := newLiveHandler(t, fetcher)

	recorder := serveLiveRequest(t, handler,
		"/orbitplus/api/3.0/json/bits/pathuser/pathtoken/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0&zone=r3bits")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", recorder.Code, recorder.Body.String())
	}
	if !fetcher.called {
		t.Fatal("expected an outbound fetch")
	}
	expectedBaseURL, _ := master.ZoneBitsBaseURL("r3bits")
	if fetcher.lookup.BaseURL != expectedBaseURL {
		t.Errorf("BaseURL = %q, want %q", fetcher.lookup.BaseURL, expectedBaseURL)
	}
	if fetcher.lookup.Username != "pathuser" || fetcher.lookup.APIToken != "pathtoken" {
		t.Errorf("credential = %q/%q, want pathuser/pathtoken", fetcher.lookup.Username, fetcher.lookup.APIToken)
	}
	if fetcher.lookup.OperatorCode != "bits" {
		t.Errorf("OperatorCode = %q, want bits", fetcher.lookup.OperatorCode)
	}
	if fetcher.lookup.FromCode != "CITY_A" || fetcher.lookup.ToCode != "CITY_B" || fetcher.lookup.TravelDate != "2026-08-21" {
		t.Errorf("route = %q/%q/%q, want CITY_A/CITY_B/2026-08-21",
			fetcher.lookup.FromCode, fetcher.lookup.ToCode, fetcher.lookup.TravelDate)
	}
}

// An unusable zone is the caller's error, so it is rejected before any outbound
// call rather than becoming a gateway error after a pointless attempt.
func TestLiveReadRejectsInvalidLiveRequest(t *testing.T) {
	for name, requestPath := range map[string]string{
		"absent_zone": "/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0",
		"unknown_zone": "/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0&zone=nosuchzone",
		"blank_zone":   "/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0&zone=",
		"repeated_zone":    "/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0&zone=bits&zone=r2bits",
		"undecodable_zone": "/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=0&zone=%zz",
	} {
		t.Run(name, func(t *testing.T) {
			fetcher := &recordingFetcher{}
			handler := newLiveHandler(t, fetcher)

			recorder := serveLiveRequest(t, handler, requestPath)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", recorder.Code)
			}
			if fetcher.called {
				t.Error("no outbound fetch should be issued for an invalid live request")
			}
		})
	}
}

// A cached read needs no zone and must not consult Bits.
func TestCachedReadIgnoresZone(t *testing.T) {
	fetcher := &recordingFetcher{}
	handler := newLiveHandler(t, fetcher)

	recorder := serveLiveRequest(t, handler,
		"/orbitplus/api/3.0/json/bits/u/t/search/CITY_A/CITY_B/2026-08-21?cacheFlag=1&zone=nosuchzone")

	// The read service is nil in this harness, so the cached path reports a
	// server error. The assertion that matters is that nothing went upstream.
	if recorder.Code == http.StatusBadRequest {
		t.Error("an unknown zone must not fail a cached read")
	}
	if fetcher.called {
		t.Error("a cached read must not issue an outbound fetch")
	}
}
