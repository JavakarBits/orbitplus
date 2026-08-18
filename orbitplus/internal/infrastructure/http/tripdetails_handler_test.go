package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"orbitplusmaster/internal/application/master"
)

func TestTripDetailsHandlerAcceptsSchemaAgnosticJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"worker envelope", `{"actionType":"search","operatorCode":"bits","fromCode":"FROM","toCode":"TO","tripDate":"2026-08-20","orbitResponse":{"status":1,"data":[]}}`},
		{"array", `[{"tripCode":"ABC"}]`},
		{"string", `"test"`},
		{"number", `12345678901234567890`},
		{"boolean", `true`},
		{"null", `null`},
		{"missing action type", `{"data":[]}`},
		{"unknown action type", `{"actionType":"unknown","data":[]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs, recorder := serveTripDetails(test.body, nil)
			assertJSONStatus(t, recorder, http.StatusOK, 1, "Trip details received successfully")
			assertRawPayloadLogged(t, logs, test.body)
		})
	}
}

func TestTripDetailsHandlerRejectsInvalidOrMultipleJSONValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"whitespace only", " \t\n "},
		{"malformed", `{"secret":"SENSITIVE_MARKER"`},
		{"second value", `{} {}`},
		{"trailing content", `{} garbage`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs, recorder := serveTripDetails(test.body, nil)
			assertJSONStatus(t, recorder, http.StatusBadRequest, 0, "Invalid trip details JSON")
			if !strings.Contains(logs, "TripDetails JSON validation failed") || strings.Contains(logs, "TripDetails payload:") || strings.Contains(logs, "SENSITIVE_MARKER") {
				t.Fatalf("unsafe invalid-JSON logs: %q", logs)
			}
		})
	}
}

func TestTripDetailsHandlerPreservesTrailingWhitespaceWhenLogging(t *testing.T) {
	body := "{\"actionType\":\"search\",\"data\":[]} \n\t"
	logs, recorder := serveTripDetails(body, nil)
	assertJSONStatus(t, recorder, http.StatusOK, 1, "Trip details received successfully")
	assertRawPayloadLogged(t, logs, body)
}

func TestTripDetailsHandlerHandlesBodyReadFailureSafely(t *testing.T) {
	logs, recorder := serveTripDetails("SENSITIVE_BODY", errors.New("read failed"))
	assertJSONStatus(t, recorder, http.StatusInternalServerError, 0, "Internal server error")
	if !strings.Contains(logs, "TripDetails request body read failed") || strings.Contains(logs, "SENSITIVE_BODY") || strings.Contains(logs, "TripDetails payload:") {
		t.Fatalf("unsafe body-read-failure logs: %q", logs)
	}
}

func TestRouterHealthEndpoint(t *testing.T) {
	router := NewRouter(master.NewTripDetailsService())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != `{"status":"UP"}` {
		t.Fatalf("body = %q, want health response", recorder.Body.String())
	}
}

func serveTripDetails(body string, readError error) (string, *httptest.ResponseRecorder) {
	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	service := master.NewTripDetailsServiceWithLogger(logger)
	handler := newTripDetailsHandler(service, logger)
	request := httptest.NewRequest(http.MethodPost, "/api/tripdetails", nil)
	if readError == nil {
		request.Body = io.NopCloser(strings.NewReader(body))
	} else {
		request.Body = failingBody{err: readError}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return logs.String(), recorder
}

type failingBody struct{ err error }

func (body failingBody) Read([]byte) (int, error) { return 0, body.err }
func (failingBody) Close() error                  { return nil }

func assertJSONStatus(t *testing.T, recorder *httptest.ResponseRecorder, expectedStatus, expectedValue int, expectedMessage string) {
	t.Helper()
	if recorder.Code != expectedStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, expectedStatus, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", recorder.Header().Get("Content-Type"))
	}
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Status != expectedValue || response.Message != expectedMessage {
		t.Fatalf("response = %#v, want status=%d message=%q", response, expectedValue, expectedMessage)
	}
}

func assertRawPayloadLogged(t *testing.T, logs, rawBody string) {
	t.Helper()
	want := "TripDetails request received\nTripDetails payload: " + rawBody + "\nTripDetails request completed successfully\n"
	if logs != want {
		t.Fatalf("logs = %q, want raw payload log %q", logs, want)
	}
}
