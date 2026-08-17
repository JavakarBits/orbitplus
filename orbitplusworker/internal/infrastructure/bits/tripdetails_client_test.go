package bits_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orbitplusworker/internal/application/worker"
	"orbitplusworker/internal/domain"
	"orbitplusworker/internal/infrastructure/bits"
)

func TestFetchTripDetails_SearchRoute(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"data":[]}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "CITY_A",
			ToCode:       "CITY_B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN123",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	expectedPath := "/busservices/api/3.0/json/OP1/ram/TOKEN123/search/CITY_A/CITY_B/2026-08-20"
	if receivedPath != expectedPath {
		t.Errorf("path mismatch:\n  got:  %s\n  want: %s", receivedPath, expectedPath)
	}
}

func TestFetchTripDetails_BusmapRoute(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:      "busmap",
			OperatorCode:    "OP1",
			TripCode:        "TRIP_X",
			FromStationCode: "STN_FROM",
			ToStationCode:   "STN_TO",
			TravelDate:      "2026-08-25",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN123",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	expectedPath := "/busservices/api/3.0/json/OP1/ram/TOKEN123/busmap/TRIP_X/STN_FROM/STN_TO/2026-08-25"
	if receivedPath != expectedPath {
		t.Errorf("path mismatch:\n  got:  %s\n  want: %s", receivedPath, expectedPath)
	}
}

func TestFetchTripDetails_SearchbusmapRoute(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "searchbusmap",
			OperatorCode: "OP1",
			FromCode:     "FROM_C",
			ToCode:       "TO_D",
			TripDate:     "2026-09-01",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN123",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	expectedPath := "/busservices/api/3.0/json/OP1/ram/TOKEN123/search/busmap/FROM_C/TO_D/2026-09-01"
	if receivedPath != expectedPath {
		t.Errorf("path mismatch:\n  got:  %s\n  want: %s", receivedPath, expectedPath)
	}
}

func TestFetchTripDetails_DynamicSegmentsArePathEscaped(t *testing.T) {
	var receivedRawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRawPath = r.URL.RawPath
		if receivedRawPath == "" {
			receivedRawPath = r.URL.Path
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	// Use special characters that require path escaping
	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP/1",
			FromCode:     "FROM A",
			ToCode:       "TO#B",
			TripDate:     "2026/08/20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP/1",
			Username:     "user name",
			APIToken:     "token/key",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	// Verify special characters are escaped in the raw path
	if strings.Contains(receivedRawPath, " ") {
		t.Error("raw path contains unescaped space")
	}
	if strings.Contains(receivedRawPath, "#") {
		t.Error("raw path contains unescaped #")
	}
	// Verify the escaped segments are present
	if !strings.Contains(receivedRawPath, "OP%2F1") {
		t.Errorf("operatorCode not properly escaped in path: %s", receivedRawPath)
	}
	if !strings.Contains(receivedRawPath, "FROM%20A") {
		t.Errorf("fromCode not properly escaped in path: %s", receivedRawPath)
	}
	if !strings.Contains(receivedRawPath, "TO%23B") {
		t.Errorf("toCode not properly escaped in path: %s", receivedRawPath)
	}
	if !strings.Contains(receivedRawPath, "user%20name") {
		t.Errorf("username not properly escaped in path: %s", receivedRawPath)
	}
	if !strings.Contains(receivedRawPath, "token%2Fkey") {
		t.Errorf("apiToken not properly escaped in path: %s", receivedRawPath)
	}
}

func TestFetchTripDetails_ReturnsRawBody(t *testing.T) {
	expectedBody := `{"status":1,"datetime":"2026-08-13","data":[{"tripCode":"T1","fare":2999}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedBody))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	resp, err := client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if string(resp.Body) != expectedBody {
		t.Errorf("body mismatch:\n  got:  %s\n  want: %s", resp.Body, expectedBody)
	}
}

func TestFetchTripDetails_NonSuccessHTTPReturnsError(t *testing.T) {
	statusCodes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	}

	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`error`))
			}))
			defer server.Close()

			client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
			if err != nil {
				t.Fatalf("NewBitsTripDetailsClient: %v", err)
			}

			req := worker.BitsTripDetailsRequest{
				Message: domain.TripDetailsRefreshMessage{
					ActionType:   "search",
					OperatorCode: "OP1",
					FromCode:     "A",
					ToCode:       "B",
					TripDate:     "2026-08-20",
				},
				Credential: worker.BitsOperatorCredential{
					OperatorCode: "OP1",
					Username:     "ram",
					APIToken:     "TOKEN",
					BaseURL:      server.URL,
				},
			}

			_, err = client.FetchTripDetails(context.Background(), req)
			if err == nil {
				t.Fatalf("expected error for HTTP %d, got nil", code)
			}
		})
	}
}

func TestFetchTripDetails_UsesGETMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if receivedMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", receivedMethod)
	}
}

func TestFetchTripDetails_SetsAcceptJSON(t *testing.T) {
	var receivedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if receivedAccept != "application/json" {
		t.Errorf("expected Accept: application/json, got %q", receivedAccept)
	}
}

func TestFetchTripDetails_DoesNotSendCredentialsInHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "SECRET_TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}

	// Credentials must only be in the URL path, never in headers
	if auth := receivedHeaders.Get("Authorization"); auth != "" {
		t.Errorf("unexpected Authorization header: %q", auth)
	}
	// Check no header value contains the API token
	for name, values := range receivedHeaders {
		for _, value := range values {
			if strings.Contains(value, "SECRET_TOKEN") {
				t.Errorf("header %q contains credential: %q", name, value)
			}
		}
	}
}

func TestFetchTripDetails_DoesNotSendRequestBody(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("FetchTripDetails: %v", err)
	}
	if len(receivedBody) != 0 {
		t.Errorf("expected no request body, got %d bytes", len(receivedBody))
	}
}

func TestFetchTripDetails_InvalidMessageReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server for invalid message")
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	// Missing fromCode for search
	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "OP1",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid message")
	}
}

func TestFetchTripDetails_InvalidCredentialReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server for invalid credential")
	}))
	defer server.Close()

	client, err := bits.NewBitsTripDetailsClient(server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}

	// Credential operator mismatch
	req := worker.BitsTripDetailsRequest{
		Message: domain.TripDetailsRefreshMessage{
			ActionType:   "search",
			OperatorCode: "OP1",
			FromCode:     "A",
			ToCode:       "B",
			TripDate:     "2026-08-20",
		},
		Credential: worker.BitsOperatorCredential{
			OperatorCode: "WRONG_OP",
			Username:     "ram",
			APIToken:     "TOKEN",
			BaseURL:      server.URL,
		},
	}

	_, err = client.FetchTripDetails(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for mismatched credential operator")
	}
}
