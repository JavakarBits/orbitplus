package orbitplus_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"orbitplusworker/internal/application/worker"
	"orbitplusworker/internal/domain"
	"orbitplusworker/internal/infrastructure/orbitplus"
)

func validSearchMessage() domain.TripDetailsRefreshMessage {
	return domain.TripDetailsRefreshMessage{
		ActionType:   "search",
		OperatorCode: "OP1",
		FromCode:     "CITY_A",
		ToCode:       "CITY_B",
		TripDate:     "2026-08-20",
	}
}

func validBitsResponse() []byte {
	return []byte(`{"status":1,"data":[{"tripCode":"T1","fare":2999}]}`)
}

func TestSendTripDetails_PostsToApiTripdetails(t *testing.T) {
	var receivedPath string
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	req := worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	}

	_, err = client.SendTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("SendTripDetails: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if receivedPath != "/api/tripdetails" {
		t.Errorf("expected /api/tripdetails, got %s", receivedPath)
	}
}

func TestSendTripDetails_SendsOrbitResponseFieldUnchanged(t *testing.T) {
	bitsJSON := []byte(`{"status":1,"datetime":"2026-08-13","data":[{"fare":2999,"seatType":"LSL","special":"café"}]}`)
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	req := worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: bitsJSON,
	}

	_, err = client.SendTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("SendTripDetails: %v", err)
	}

	// Parse the sent payload and verify orbitResponse is the raw Bits JSON
	var payload struct {
		OrbitResponse json.RawMessage `json:"orbitResponse"`
	}
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if string(payload.OrbitResponse) != string(bitsJSON) {
		t.Errorf("orbitResponse mismatch:\n  got:  %s\n  want: %s", payload.OrbitResponse, bitsJSON)
	}
}

func TestSendTripDetails_IncludesMessageFields(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	msg := domain.TripDetailsRefreshMessage{
		ActionType:      "busmap",
		OperatorCode:    "OPERATOR_X",
		TripCode:        "TRIP_ABC",
		FromStationCode: "STN_A",
		ToStationCode:   "STN_B",
		TravelDate:      "2026-09-15",
	}
	req := worker.TripDetailsRefreshRequest{
		Message:      msg,
		BitsResponse: validBitsResponse(),
	}

	_, err = client.SendTripDetails(context.Background(), req)
	if err != nil {
		t.Fatalf("SendTripDetails: %v", err)
	}

	var payload struct {
		ActionType      string `json:"actionType"`
		OperatorCode    string `json:"operatorCode"`
		TripCode        string `json:"tripCode"`
		FromStationCode string `json:"fromStationCode"`
		ToStationCode   string `json:"toStationCode"`
		TravelDate      string `json:"travelDate"`
	}
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ActionType != msg.ActionType {
		t.Errorf("actionType: got %q, want %q", payload.ActionType, msg.ActionType)
	}
	if payload.OperatorCode != msg.OperatorCode {
		t.Errorf("operatorCode: got %q, want %q", payload.OperatorCode, msg.OperatorCode)
	}
	if payload.TripCode != msg.TripCode {
		t.Errorf("tripCode: got %q, want %q", payload.TripCode, msg.TripCode)
	}
	if payload.FromStationCode != msg.FromStationCode {
		t.Errorf("fromStationCode: got %q, want %q", payload.FromStationCode, msg.FromStationCode)
	}
	if payload.ToStationCode != msg.ToStationCode {
		t.Errorf("toStationCode: got %q, want %q", payload.ToStationCode, msg.ToStationCode)
	}
	if payload.TravelDate != msg.TravelDate {
		t.Errorf("travelDate: got %q, want %q", payload.TravelDate, msg.TravelDate)
	}
}

func TestSendTripDetails_Status1ReturnsAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	status, err := client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	})
	if err != nil {
		t.Fatalf("SendTripDetails: %v", err)
	}
	if status != worker.OrbitPlusAccepted {
		t.Errorf("expected ACCEPTED, got %s", status)
	}
}

func TestSendTripDetails_RetryableHTTPCodes(t *testing.T) {
	codes := []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			status, err := client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
				Message:      validSearchMessage(),
				BitsResponse: validBitsResponse(),
			})
			if err != nil {
				t.Fatalf("SendTripDetails should not error for retryable codes: %v", err)
			}
			if status != worker.OrbitPlusRetryable {
				t.Errorf("expected RETRYABLE for HTTP %d, got %s", code, status)
			}
		})
	}
}

func TestSendTripDetails_NonRetryableErrorCodes(t *testing.T) {
	codes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer server.Close()

			client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
				Message:      validSearchMessage(),
				BitsResponse: validBitsResponse(),
			})
			if err == nil {
				t.Fatalf("expected error for HTTP %d", code)
			}
		})
	}
}

func TestSendTripDetails_UnsuccessfulStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":0,"message":"failed"}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	})
	if err == nil {
		t.Fatal("expected error for status:0 response")
	}
}

func TestSendTripDetails_NullStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"no status field"}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	})
	if err == nil {
		t.Fatal("expected error for null/missing status")
	}
}

func TestSendTripDetails_InvalidBitsJSON_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server with invalid Bits JSON")
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: []byte(`not valid json`),
	})
	if err == nil {
		t.Fatal("expected error for invalid Bits JSON")
	}
}

func TestSendTripDetails_InvalidMessage_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server with invalid message")
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Missing operatorCode
	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      domain.TripDetailsRefreshMessage{ActionType: "search"},
		BitsResponse: validBitsResponse(),
	})
	if err == nil {
		t.Fatal("expected error for invalid message")
	}
}

func TestSendTripDetails_SetsContentTypeJSON(t *testing.T) {
	var receivedContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	client, err := orbitplus.NewClient(server.URL, 64<<10, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	})
	if err != nil {
		t.Fatalf("SendTripDetails: %v", err)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %q", receivedContentType)
	}
}

func TestSendTripDetails_ResponseTooLarge_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write more than the 128-byte limit
		largeBody := make([]byte, 256)
		for i := range largeBody {
			largeBody[i] = 'x'
		}
		_, _ = w.Write(largeBody)
	}))
	defer server.Close()

	// Use a very small response limit
	client, err := orbitplus.NewClient(server.URL, 128, server.Client(), worker.Development)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.SendTripDetails(context.Background(), worker.TripDetailsRefreshRequest{
		Message:      validSearchMessage(),
		BitsResponse: validBitsResponse(),
	})
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}
