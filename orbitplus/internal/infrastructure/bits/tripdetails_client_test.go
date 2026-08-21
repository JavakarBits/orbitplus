package bits_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/bits"
)

func newTestClient(t *testing.T, server *httptest.Server) *bits.BitsTripDetailsClient {
	t.Helper()
	client, err := bits.NewBitsTripDetailsClient(server.Client(), master.VerificationConfig{
		BitsBaseURL:   server.URL,
		BitsUsername:  "ram",
		BitsAPIToken:  "TOKEN123",
		HTTPTimeout:   5 * time.Second,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}
	return client
}

func searchLookup() master.BitsLookup {
	return master.BitsLookup{
		Action:       master.BitsActionSearch,
		OperatorCode: "OP1",
		FromCode:     "CITY_A",
		ToCode:       "CITY_B",
		TravelDate:   "2026-08-20",
	}
}

func TestFetchTripDetailsSearchRoute(t *testing.T) {
	var receivedPath, receivedMethod, receivedAccept string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		receivedAccept = r.Header.Get("Accept")
		receivedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"status":1,"data":[{"tripCode":"T1"}]}`))
	}))
	defer server.Close()

	result, err := newTestClient(t, server).FetchTripDetails(context.Background(), searchLookup())
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	expectedPath := "/busservices/api/3.0/json/OP1/ram/TOKEN123/search/CITY_A/CITY_B/2026-08-20"
	if receivedPath != expectedPath {
		t.Errorf("path:\n  got:  %s\n  want: %s", receivedPath, expectedPath)
	}
	if receivedMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", receivedMethod)
	}
	if receivedAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", receivedAccept)
	}
	if len(receivedBody) != 0 {
		t.Errorf("request body = %d bytes, want 0", len(receivedBody))
	}
	if result.Empty {
		t.Error("Empty = true, want false for a populated data array")
	}
	if result.DataKind != master.BitsDataKindArray {
		t.Errorf("DataKind = %q, want %q", result.DataKind, master.BitsDataKindArray)
	}
	if string(result.Data) != `[{"tripCode":"T1"}]` {
		t.Errorf("Data = %s, want the data member verbatim", result.Data)
	}
}

func TestFetchTripDetailsBusMapRoute(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_, _ = w.Write([]byte(`{"status":1,"data":{"tripCode":"TRIP_X"}}`))
	}))
	defer server.Close()

	result, err := newTestClient(t, server).FetchTripDetails(context.Background(), master.BitsLookup{
		Action:       master.BitsActionBusMap,
		OperatorCode: "OP1",
		TripCode:     "TRIP_X",
		FromCode:     "STN_FROM",
		ToCode:       "STN_TO",
		TravelDate:   "2026-08-25",
	})
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	expectedPath := "/busservices/api/3.0/json/OP1/ram/TOKEN123/busmap/TRIP_X/STN_FROM/STN_TO/2026-08-25"
	if receivedPath != expectedPath {
		t.Errorf("path:\n  got:  %s\n  want: %s", receivedPath, expectedPath)
	}
	if result.DataKind != master.BitsDataKindObject {
		t.Errorf("DataKind = %q, want %q", result.DataKind, master.BitsDataKindObject)
	}
}

func TestFetchTripDetailsEscapesDynamicSegments(t *testing.T) {
	var receivedRawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRawPath = r.URL.RawPath
		if receivedRawPath == "" {
			receivedRawPath = r.URL.Path
		}
		_, _ = w.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), master.VerificationConfig{
		BitsBaseURL:  server.URL,
		BitsUsername: "user name",
		BitsAPIToken: "token/key",
		HTTPTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	_, err = client.FetchTripDetails(context.Background(), master.BitsLookup{
		Action:       master.BitsActionSearch,
		OperatorCode: "OP/1",
		FromCode:     "FROM A",
		ToCode:       "TO#B",
		TravelDate:   "2026/08/20",
	})
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	for _, unescaped := range []string{" ", "#"} {
		if strings.Contains(receivedRawPath, unescaped) {
			t.Errorf("raw path contains unescaped %q: %s", unescaped, receivedRawPath)
		}
	}
	for _, expected := range []string{"OP%2F1", "FROM%20A", "TO%23B", "user%20name", "token%2Fkey"} {
		if !strings.Contains(receivedRawPath, expected) {
			t.Errorf("raw path missing escaped segment %q: %s", expected, receivedRawPath)
		}
	}
}

func TestFetchTripDetailsEmptyDataForms(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "null", body: `{"status":1,"data":null}`},
		{name: "empty array", body: `{"status":1,"data":[]}`},
		{name: "empty object", body: `{"status":1,"data":{}}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			result, err := newTestClient(t, server).FetchTripDetails(context.Background(), searchLookup())
			if err != nil {
				t.Fatalf("FetchTripDetails returned %v, want nil: an empty data member is a successful fetch", err)
			}
			if !result.Empty {
				t.Error("Empty = false, want true")
			}
		})
	}
}

func TestFetchTripDetailsFailureModes(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "400", status: http.StatusBadRequest, body: `{}`},
		{name: "401", status: http.StatusUnauthorized, body: `{}`},
		{name: "500", status: http.StatusInternalServerError, body: `{}`},
		{name: "empty body", status: http.StatusOK, body: ``},
		{name: "invalid json", status: http.StatusOK, body: `{not json`},
		{name: "top level array", status: http.StatusOK, body: `[1,2,3]`},
		{name: "no data member", status: http.StatusOK, body: `{"status":1}`},
		{name: "data is a string", status: http.StatusOK, body: `{"data":"nope"}`},
		{name: "data is a number", status: http.StatusOK, body: `{"data":42}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			_, err := newTestClient(t, server).FetchTripDetails(context.Background(), searchLookup())
			if !errors.Is(err, master.ErrLiveSourceUnavailable) {
				t.Fatalf("error = %v, want ErrLiveSourceUnavailable", err)
			}
		})
	}
}

func TestFetchTripDetailsErrorNeverLeaksCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"secret":"UPSTREAM_MARKER"}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), master.VerificationConfig{
		BitsBaseURL:  server.URL,
		BitsUsername: "SECRET_USER",
		BitsAPIToken: "SECRET_TOKEN",
		HTTPTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	_, err = client.FetchTripDetails(context.Background(), searchLookup())
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, secret := range []string{"SECRET_USER", "SECRET_TOKEN", "UPSTREAM_MARKER", "403"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error text contains %q: %v", secret, err)
		}
	}
}

func TestFetchTripDetailsSendsNoCredentialHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		_, _ = w.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	if _, err := newTestClient(t, server).FetchTripDetails(context.Background(), searchLookup()); err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if auth := receivedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("Authorization header = %q, want empty: Bits credentials belong in the path", auth)
	}
	for name, values := range receivedHeaders {
		for _, value := range values {
			if strings.Contains(value, "TOKEN123") {
				t.Errorf("header %q carries the API token: %q", name, value)
			}
		}
	}
}

func TestFetchTripDetailsRejectsUnsupportedAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request should be issued for an unsupported action")
	}))
	defer server.Close()

	lookup := searchLookup()
	lookup.Action = "SEARCHBUSMAP"
	_, err := newTestClient(t, server).FetchTripDetails(context.Background(), lookup)
	if !errors.Is(err, master.ErrLiveSourceUnavailable) {
		t.Fatalf("error = %v, want ErrLiveSourceUnavailable", err)
	}
}

func TestFetchTripDetailsStripsBaseURLQueryAndFragment(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), master.VerificationConfig{
		BitsBaseURL:  server.URL + "/prefix?stray=1#frag",
		BitsUsername: "ram",
		BitsAPIToken: "TOKEN123",
		HTTPTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}
	if _, err := client.FetchTripDetails(context.Background(), searchLookup()); err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if receivedQuery != "" {
		t.Errorf("query = %q, want empty: a stray base-URL query must be discarded", receivedQuery)
	}
}

func TestFetchTripDetailsHonoursTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()
	defer close(release)

	httpClient := server.Client()
	httpClient.Timeout = 50 * time.Millisecond
	client, err := bits.NewBitsTripDetailsClient(httpClient, master.VerificationConfig{
		BitsBaseURL:  server.URL,
		BitsUsername: "ram",
		BitsAPIToken: "TOKEN123",
		HTTPTimeout:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	started := time.Now()
	_, err = client.FetchTripDetails(context.Background(), searchLookup())
	if !errors.Is(err, master.ErrLiveSourceUnavailable) {
		t.Fatalf("error = %v, want ErrLiveSourceUnavailable", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %s, want the timeout to abandon the request promptly", elapsed)
	}
}
