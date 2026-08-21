// Package orbit implements the OrbitService operator credential adapter.
package orbit

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

// OrbitClient resolves per-operator Bits credentials from OrbitService. Request
// URLs contain credential material and are never logged.
type OrbitClient struct {
	endpoint      *url.URL
	namespaceCode string
	accessToken   string
	maxResponse   int64
	client        *http.Client
}

func NewClient(endpoint, namespaceCode, accessToken string, maxResponseBytes int64, httpClient *http.Client, environment worker.AppEnvironment) (*OrbitClient, error) {
	if err := worker.ValidateOrbitURL(endpoint, environment); err != nil {
		return nil, fmt.Errorf("Orbit endpoint: %w", err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("Orbit endpoint: %w", err)
	}
	if strings.TrimSpace(namespaceCode) == "" || strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("Orbit namespace code and access token are required")
	}
	if maxResponseBytes <= 0 || httpClient == nil {
		return nil, fmt.Errorf("Orbit response limit and HTTP client are required")
	}
	return &OrbitClient{endpoint: parsed, namespaceCode: namespaceCode, accessToken: accessToken, maxResponse: maxResponseBytes, client: httpClient}, nil
}

func (client *OrbitClient) FetchOperatorCredential(ctx context.Context, request worker.OperatorCredentialRequest) (worker.OperatorCredential, error) {
	operatorCode := strings.TrimSpace(request.OperatorCode)
	if operatorCode == "" {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit request requires an operator code")
	}

	requestURL := *client.endpoint
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/orbitservices/" +
		url.PathEscape(client.namespaceCode) + "/" +
		url.PathEscape(operatorCode) + "/" +
		url.PathEscape(client.accessToken) + "/operator"

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return worker.OperatorCredential{}, fmt.Errorf("build Orbit credential request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")

	response, err := client.client.Do(httpRequest)
	if err != nil {
		// The error is not wrapped because transport errors can contain the
		// credential-bearing request URL.
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential request returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponse+1))
	if err != nil || int64(len(body)) > client.maxResponse {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential response is invalid")
	}
	var result struct {
		Status *int `json:"status"`
		Data   struct {
			Code       string `json:"code"`
			Username   string `json:"username"`
			APIToken   string `json:"apiToken"`
			ActiveFlag *int   `json:"activeFlag"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return worker.OperatorCredential{}, fmt.Errorf("decode Orbit credential response: %w", err)
	}
	if result.Status == nil || *result.Status != 1 {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential response reported an unsuccessful status")
	}
	if result.Data.ActiveFlag == nil || *result.Data.ActiveFlag != 1 {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit operator is not active")
	}
	if strings.TrimSpace(result.Data.Username) == "" {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential response has no Bits username")
	}
	if strings.TrimSpace(result.Data.APIToken) == "" {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential response has no API token")
	}
	if !strings.EqualFold(strings.TrimSpace(result.Data.Code), operatorCode) {
		return worker.OperatorCredential{}, fmt.Errorf("Orbit credential response operator code does not match the requested operator")
	}
	return worker.OperatorCredential{
		OperatorCode: operatorCode,
		Username:     result.Data.Username,
		APIToken:     result.Data.APIToken,
	}, nil
}

var _ worker.OperatorCredentialClient = (*OrbitClient)(nil)
