// Package orbitplus implements the Worker-only OrbitPlus submission adapter.
package orbitplus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"orbitplusworker/internal/application/worker"
)

// OrbitPlusClient sends refresh candidates exclusively to OrbitPlus.
type OrbitPlusClient struct {
	endpoint    *url.URL
	maxResponse int64
	client      *http.Client
}

func NewClient(endpoint string, maxResponseBytes int64, httpClient *http.Client, environment worker.AppEnvironment) (*OrbitPlusClient, error) {
	parsed, err := parseEndpoint(endpoint, environment)
	if err != nil {
		return nil, fmt.Errorf("OrbitPlus endpoint: %w", err)
	}
	if maxResponseBytes <= 0 || httpClient == nil {
		return nil, fmt.Errorf("OrbitPlus response limit and HTTP client are required")
	}
	return &OrbitPlusClient{endpoint: parsed, maxResponse: maxResponseBytes, client: httpClient}, nil
}

func (client *OrbitPlusClient) SendTripDetails(ctx context.Context, request worker.TripDetailsRefreshRequest) (worker.OrbitPlusStatus, error) {
	if err := request.Message.Validate(); err != nil {
		return "", fmt.Errorf("OrbitPlus request is invalid: %w", err)
	}
	if !json.Valid(request.BitsResponse) {
		return "", fmt.Errorf("OrbitPlus request has invalid Bits JSON")
	}
	payload := struct {
		ActionType      string          `json:"actionType"`
		RefID           string          `json:"refid,omitempty"`
		OperatorCode    string          `json:"operatorCode"`
		FromCode        string          `json:"fromCode,omitempty"`
		ToCode          string          `json:"toCode,omitempty"`
		TripDate        string          `json:"tripDate,omitempty"`
		TripCode        string          `json:"tripCode,omitempty"`
		FromStationCode string          `json:"fromStationCode,omitempty"`
		ToStationCode   string          `json:"toStationCode,omitempty"`
		TravelDate      string          `json:"travelDate,omitempty"`
		OrbitResponse   json.RawMessage `json:"orbitResponse"`
	}{
		ActionType:      request.Message.ActionType,
		RefID:           request.Message.ReferenceID,
		OperatorCode:    request.Message.OperatorCode,
		FromCode:        request.Message.FromCode,
		ToCode:          request.Message.ToCode,
		TripDate:        request.Message.TripDate,
		TripCode:        request.Message.TripCode,
		FromStationCode: request.Message.FromStationCode,
		ToStationCode:   request.Message.ToStationCode,
		TravelDate:      request.Message.TravelDate,
		// orbitResponse is preserved because it is the existing OrbitPlus/Master API field.
		OrbitResponse: request.BitsResponse,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode OrbitPlus request: %w", err)
	}
	requestURL := *client.endpoint
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/api/tripdetails"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build OrbitPlus request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	response, err := client.client.Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("OrbitPlus request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return worker.OrbitPlusRetryable, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OrbitPlus returned HTTP %d", response.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(response.Body, client.maxResponse+1))
	if err != nil || int64(len(body)) > client.maxResponse {
		return "", fmt.Errorf("OrbitPlus response is invalid")
	}
	var result struct {
		Status *int `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode OrbitPlus response: %w", err)
	}
	if result.Status == nil || *result.Status != 1 {
		return "", fmt.Errorf("OrbitPlus returned unsuccessful status")
	}
	return worker.OrbitPlusAccepted, nil
}

func parseEndpoint(raw string, environment worker.AppEnvironment) (*url.URL, error) {
	if err := worker.ValidateOrbitPlusURL(raw, environment); err != nil {
		return nil, err
	}
	return url.Parse(raw)
}

var _ worker.OrbitPlusClient = (*OrbitPlusClient)(nil)
