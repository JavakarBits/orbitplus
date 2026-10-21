// Package bits implements the authorized Bits source adapter.
package bits

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"orbitplusworker/internal/application/worker"
	"orbitplusworker/internal/domain"
)

// BitsTripDetailsClient fetches source data using credentials held only in
// memory for the current operation.
type BitsTripDetailsClient struct {
	client      *http.Client
	environment worker.AppEnvironment
}

func NewBitsTripDetailsClient(httpClient *http.Client, environment worker.AppEnvironment) (*BitsTripDetailsClient, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("Bits HTTP client is required")
	}
	if err := environment.Validate(); err != nil {
		return nil, err
	}
	return &BitsTripDetailsClient{client: httpClient, environment: environment}, nil
}

func (client *BitsTripDetailsClient) FetchTripDetails(ctx context.Context, request worker.BitsTripDetailsRequest) (worker.BitsTripDetailsResponse, error) {
	message := request.Message
	if err := message.Validate(); err != nil {
		return worker.BitsTripDetailsResponse{}, err
	}
	if err := request.Credential.Validate(message.OperatorCode); err != nil {
		return worker.BitsTripDetailsResponse{}, err
	}
	if err := worker.ValidateBitsURL(request.Credential.BaseURL, client.environment); err != nil {
		return worker.BitsTripDetailsResponse{}, err
	}

	baseURL, err := url.Parse(request.Credential.BaseURL)
	if err != nil {
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("parse Bits base URL: %w", err)
	}
	requestURL, err := bitsRequestURL(baseURL, message, request.Credential)
	if err != nil {
		return worker.BitsTripDetailsResponse{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("build Bits request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")

	slog.Info("Bits HTTP request started",
		"actionType", string(message.ActionType),
		"operator", message.OperatorCode,
		"host", baseURL.Host,
		"path", redactBitsPath(requestURL, request.Credential),
	)
	// WARNING: this logs the FULL BITS request URL, which embeds the operator's
	// username and apiToken in cleartext. Enabled per explicit request for
	// debugging. Remove or redact before using in a shared/production
	// environment, since worker logs are also exposed via the admin Logs UI.
	slog.Info("Bits HTTP full request",
		"actionType", string(message.ActionType),
		"operator", message.OperatorCode,
		"method", http.MethodGet,
		"url", requestURL.String(),
		"username", request.Credential.Username,
		"apiToken", request.Credential.APIToken,
	)
	response, err := client.client.Do(httpRequest)
	if err != nil {
		slog.Info("Bits HTTP request completed",
			"actionType", string(message.ActionType),
			"operator", message.OperatorCode,
			"host", baseURL.Host,
			"success", false,
			"reason", "transport_error",
		)
		return worker.BitsTripDetailsResponse{}, worker.NewRetryableError("Bits request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
		slog.Info("Bits HTTP request completed",
			"actionType", string(message.ActionType),
			"operator", message.OperatorCode,
			"host", baseURL.Host,
			"success", false,
			"status", response.StatusCode,
			"reason", "retryable_status",
		)
		return worker.BitsTripDetailsResponse{}, worker.NewRetryableError(fmt.Sprintf("Bits returned HTTP %d", response.StatusCode))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		slog.Info("Bits HTTP request completed",
			"actionType", string(message.ActionType),
			"operator", message.OperatorCode,
			"host", baseURL.Host,
			"success", false,
			"status", response.StatusCode,
			"reason", "non_success_status",
		)
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("Bits returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Info("Bits HTTP request completed",
			"actionType", string(message.ActionType),
			"operator", message.OperatorCode,
			"host", baseURL.Host,
			"success", false,
			"reason", "read_body_error",
		)
		return worker.BitsTripDetailsResponse{}, worker.NewRetryableError("Bits response could not be read")
	}
	slog.Info("Bits HTTP request completed",
		"actionType", string(message.ActionType),
		"operator", message.OperatorCode,
		"host", baseURL.Host,
		"success", true,
		"status", response.StatusCode,
		"responseBytes", len(body),
		"responseBody", truncateForLog(body, maxBitsBodyLog),
	)
	return worker.BitsTripDetailsResponse{Body: append([]byte(nil), body...)}, nil
}

// maxBitsBodyLog bounds how much of a BITS response body is written to the log,
// so a large payload cannot flood the log file. The response is trip data, not
// a secret, so logging a bounded prefix is safe and useful for debugging.
const maxBitsBodyLog = 4096

// truncateForLog returns body as a string, trimmed to max bytes with a marker
// so the log stays bounded.
func truncateForLog(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}

// redactBitsPath returns the BITS request path with the secret segments
// (username and apiToken) replaced by "***", so the log shows which route was
// called without ever writing credentials. It never returns the raw path when
// the secrets could not be located, to avoid leaking them by accident.
func redactBitsPath(requestURL *url.URL, credential worker.BitsOperatorCredential) string {
	path := requestURL.EscapedPath()
	if credential.Username != "" {
		path = strings.ReplaceAll(path, url.PathEscape(credential.Username), "***")
	}
	if credential.APIToken != "" {
		path = strings.ReplaceAll(path, url.PathEscape(credential.APIToken), "***")
	}
	return path
}

func bitsRequestURL(baseURL *url.URL, message domain.TripDetailsRefreshMessage, credential worker.BitsOperatorCredential) (*url.URL, error) {
	segments := []string{"busservices", "api", "3.0", "json", credential.OperatorCode, credential.Username, credential.APIToken}
	switch message.ActionType {
	case domain.ActionSearch:
		segments = append(segments, "search", message.FromCode, message.ToCode, message.TripDate)
	case domain.ActionBusMap:
		segments = append(segments, "busmap", message.TripCode, message.FromStationCode, message.ToStationCode, message.TravelDate)
	case domain.ActionSearchBusMap:
		segments = append(segments, "search", "busmap", message.FromCode, message.ToCode, message.TripDate)
	default:
		return nil, fmt.Errorf("unsupported actionType: %s", message.ActionType)
	}

	requestURL := *baseURL
	requestURL.User = nil
	requestURL.RawQuery = ""
	requestURL.Fragment = ""

	// Build the escaped path first, then derive the decoded Path from it so
	// that Go's url.URL.RequestURI() consistently uses the escaped form.
	escapedBase := strings.TrimRight(requestURL.EscapedPath(), "/")
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	requestURL.RawPath = escapedBase + "/" + strings.Join(escaped, "/")

	// Derive Path by unescaping RawPath so the two are consistent.
	decoded, err := url.PathUnescape(requestURL.RawPath)
	if err != nil {
		return nil, fmt.Errorf("unescape Bits path: %w", err)
	}
	requestURL.Path = decoded
	return &requestURL, nil
}

var _ worker.TripDetailsClient = (*BitsTripDetailsClient)(nil)
