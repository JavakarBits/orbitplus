// Package bits implements the authorized Bits source adapter.
package bits

import (
	"context"
	"fmt"
	"io"
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

	response, err := client.client.Do(httpRequest)
	if err != nil {
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("Bits request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("Bits returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return worker.BitsTripDetailsResponse{}, fmt.Errorf("read Bits response: %w", err)
	}
	return worker.BitsTripDetailsResponse{Body: append([]byte(nil), body...)}, nil
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
