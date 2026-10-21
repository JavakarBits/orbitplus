// Package mastercred resolves operator BITS credentials from the OrbitPlus
// Master instead of calling the Orbit credential service directly. The Master
// is the single component that talks to Orbit; workers ask the Master.
package mastercred

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"orbitplusworker/internal/application/worker"
)

// Client fetches an operator's BITS credential from the Master's internal
// credential endpoint:
//
//	GET {baseURL}/orbitplus/api/internal/operators/{operatorCode}/credential
//
// Authentication (the shared internal bearer token) is supplied by the injected
// http.Client's transport, matching how the OrbitPlus client is authenticated.
type Client struct {
	endpoint    *url.URL
	maxResponse int64
	client      *http.Client
}

// NewClient constructs the Master credential client. baseURL is the Master's
// base URL (ORBITPLUS_URL); the credential path is appended per request.
func NewClient(baseURL string, maxResponseBytes int64, httpClient *http.Client, environment worker.AppEnvironment) (*Client, error) {
	if err := worker.ValidateOrbitPlusURL(baseURL, environment); err != nil {
		return nil, fmt.Errorf("Master credential endpoint: %w", err)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("Master credential endpoint: %w", err)
	}
	if maxResponseBytes <= 0 || httpClient == nil {
		return nil, fmt.Errorf("Master credential response limit and HTTP client are required")
	}
	return &Client{endpoint: parsed, maxResponse: maxResponseBytes, client: httpClient}, nil
}

type credentialResponse struct {
	OperatorCode string `json:"operatorCode"`
	ZoneCode     string `json:"zoneCode"`
	ZoneURL      string `json:"zoneUrl"`
	Username     string `json:"username"`
	APIToken     string `json:"apiToken"`
	ActiveFlag   int    `json:"activeFlag"`
}

// FetchOperatorCredential resolves one operator's BITS credential from the
// Master. Transport errors and 408/429/5xx are returned as retryable so the
// worker's bounded-retry loop treats a transiently unavailable Master the same
// way it treated a transiently unavailable Orbit service.
func (client *Client) FetchOperatorCredential(ctx context.Context, request worker.OperatorCredentialRequest) (worker.OperatorCredential, error) {
	operatorCode := strings.TrimSpace(request.OperatorCode)
	if operatorCode == "" {
		return worker.OperatorCredential{}, fmt.Errorf("Master credential request requires an operator code")
	}

	requestURL := *client.endpoint
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/orbitplus/api/internal/operators/" +
		url.PathEscape(operatorCode) + "/credential"

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return worker.OperatorCredential{}, fmt.Errorf("build Master credential request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")

	response, err := client.client.Do(httpRequest)
	if err != nil {
		return worker.OperatorCredential{}, worker.NewRetryableError("Master credential request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		return worker.OperatorCredential{}, worker.NewRetryableError(fmt.Sprintf("Master credential request returned HTTP %d", response.StatusCode))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return worker.OperatorCredential{}, fmt.Errorf("Master credential request returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponse+1))
	if err != nil {
		return worker.OperatorCredential{}, worker.NewRetryableError("Master credential response could not be read")
	}
	if int64(len(body)) > client.maxResponse {
		return worker.OperatorCredential{}, fmt.Errorf("Master credential response is too large")
	}

	// The Master wraps data under a "data" member (writeJSONData); accept either
	// a bare object or a {"data": {...}} envelope for robustness.
	var envelope struct {
		Data *credentialResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return worker.OperatorCredential{}, fmt.Errorf("decode Master credential response")
	}
	result := envelope.Data
	if result == nil {
		var bare credentialResponse
		if err := json.Unmarshal(body, &bare); err != nil {
			return worker.OperatorCredential{}, fmt.Errorf("decode Master credential response")
		}
		result = &bare
	}
	if strings.TrimSpace(result.Username) == "" || strings.TrimSpace(result.APIToken) == "" {
		return worker.OperatorCredential{}, fmt.Errorf("Master credential response is incomplete")
	}
	if !strings.EqualFold(strings.TrimSpace(result.OperatorCode), operatorCode) {
		return worker.OperatorCredential{}, fmt.Errorf("Master credential response operator code does not match the requested operator")
	}
	return worker.OperatorCredential{
		OperatorCode: operatorCode,
		ZoneURL:      strings.TrimSpace(result.ZoneURL),
		Username:     result.Username,
		APIToken:     result.APIToken,
	}, nil
}

var _ worker.OperatorCredentialClient = (*Client)(nil)
