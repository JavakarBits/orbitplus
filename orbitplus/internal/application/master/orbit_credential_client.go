package master

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"orbitplusmaster/internal/domain"
)

// maxOrbitCredentialResponseBytes bounds how much of the Orbit credential
// response is read, matching the worker's former limit.
const maxOrbitCredentialResponseBytes = 64 << 10

// OrbitCredentialClient resolves an operator's BITS credential from the Orbit
// credential service. This is the only place the Master calls Orbit for
// credentials; the result is stored and served to workers.
//
// Credential-bearing request URLs and the resolved secret values are never
// logged.
type OrbitCredentialClient struct {
	baseURL       *url.URL
	namespaceCode string
	accessToken   string
	environment   AppEnvironment
	client        *http.Client
}

// NewOrbitCredentialClient constructs the client from a validated config. It
// returns nil when config is nil so callers can treat an unconfigured feature
// as "disabled" without a special error. environment decides the default
// scheme for the operator's zone host (http is permitted only outside
// production).
func NewOrbitCredentialClient(config *OrbitCredentialConfig, environment AppEnvironment) (*OrbitCredentialClient, error) {
	if config == nil {
		return nil, nil
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Orbit credential base URL")
	}
	return &OrbitCredentialClient{
		baseURL:       parsed,
		namespaceCode: config.NamespaceCode,
		accessToken:   config.AccessToken,
		environment:   environment,
		client:        &http.Client{Timeout: config.Timeout},
	}, nil
}

// FetchOperatorCredential resolves one operator's BITS credential. The returned
// OperatorCredential carries the operator's username/apiToken and active flag.
func (client *OrbitCredentialClient) FetchOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, error) {
	operatorCode = strings.TrimSpace(operatorCode)
	if operatorCode == "" {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential request requires an operator code")
	}

	requestURL := *client.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/orbitservices/" +
		url.PathEscape(client.namespaceCode) + "/" +
		url.PathEscape(operatorCode) + "/" +
		url.PathEscape(client.accessToken) + "/operator"

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return domain.OperatorCredential{}, fmt.Errorf("build Orbit credential request")
	}
	httpRequest.Header.Set("Accept", "application/json")

	response, err := client.client.Do(httpRequest)
	if err != nil {
		// The underlying error can embed the credential-bearing request URL, so
		// it is deliberately discarded.
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential request returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxOrbitCredentialResponseBytes+1))
	if err != nil {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response could not be read")
	}
	if int64(len(body)) > maxOrbitCredentialResponseBytes {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response is too large")
	}

	var result struct {
		Status *int `json:"status"`
		Data   struct {
			Code       string `json:"code"`
			Username   string `json:"username"`
			APIToken   string `json:"apiToken"`
			ActiveFlag *int   `json:"activeFlag"`
			// Zone carries the operator's BITS host. zone.name is the host
			// (e.g. "app.rathimeenatravels.in"), with no scheme and no
			// "/busservices" suffix; the worker appends the BITS path.
			Zone struct {
				Code string `json:"code"`
				Name string `json:"name"`
			} `json:"zone"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return domain.OperatorCredential{}, fmt.Errorf("decode Orbit credential response")
	}
	if result.Status == nil || *result.Status != 1 {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response reported an unsuccessful status")
	}
	if strings.TrimSpace(result.Data.Username) == "" {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response has no BITS username")
	}
	if strings.TrimSpace(result.Data.APIToken) == "" {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response has no API token")
	}
	if !strings.EqualFold(strings.TrimSpace(result.Data.Code), operatorCode) {
		return domain.OperatorCredential{}, fmt.Errorf("Orbit credential response operator code does not match the requested operator")
	}
	activeFlag := 0
	if result.Data.ActiveFlag != nil {
		activeFlag = *result.Data.ActiveFlag
	}
	return domain.OperatorCredential{
		OperatorCode: operatorCode,
		ZoneCode:     strings.TrimSpace(result.Data.Zone.Code),
		ZoneURL:      client.normalizeZoneHost(result.Data.Zone.Name),
		Username:     result.Data.Username,
		APIToken:     result.Data.APIToken,
		ActiveFlag:   activeFlag,
	}, nil
}

// normalizeZoneHost turns the Orbit zone.name (a bare host such as
// "app.rathimeenatravels.in", possibly already carrying a scheme or a trailing
// slash) into a scheme-qualified base URL with no trailing slash, so the worker
// can append "/busservices/...". When no scheme is present it defaults to
// https, except outside production (development) where http is used, matching
// the worker's BITS URL policy.
func (client *OrbitCredentialClient) normalizeZoneHost(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if !strings.Contains(name, "://") {
		// Always default to https for the operator's BITS zone URL, including in
		// development, since the BITS hosts are served over https.
		name = "https://" + name
	}
	return strings.TrimRight(name, "/")
}
