// Package bits implements master's outbound Bits adapter.
//
// This is master's first outbound upstream dependency. The URL mechanics here
// deliberately mirror orbitplusworker's adapter, because the subtle parts
// (setting both Path and RawPath so per-segment escaping survives, and clearing
// userinfo, query, and fragment from the configured base) were earned by
// debugging. The two modules share no code, so the mechanics are copied rather
// than imported.
package bits

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"orbitplusmaster/internal/application/master"
)

// maxBitsResponseBytes bounds how much of a Bits response is read, so a
// pathological upstream cannot exhaust master's memory.
const maxBitsResponseBytes int64 = 8 << 20

// BitsTripDetailsClient fetches a live TripDetails copy from Bits.
//
// Credentials are unexported fields and appear in exactly one place: the
// escaped path segments of an outbound request. They are never logged and never
// formatted into an error.
type BitsTripDetailsClient struct {
	client      *http.Client
	baseURL     *url.URL
	username    string
	apiToken    string
	maxBodySize int64
	logger      *log.Logger
}

// NewBitsTripDetailsClient constructs the adapter from validated configuration.
func NewBitsTripDetailsClient(httpClient *http.Client, config master.VerificationConfig) (*BitsTripDetailsClient, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("Bits HTTP client is required")
	}
	if strings.TrimSpace(config.BitsUsername) == "" || strings.TrimSpace(config.BitsAPIToken) == "" {
		return nil, fmt.Errorf("Bits credentials are required")
	}
	baseURL, err := url.Parse(config.BitsBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Bits base URL: %w", err)
	}
	return &BitsTripDetailsClient{
		client:      httpClient,
		baseURL:     baseURL,
		username:    config.BitsUsername,
		apiToken:    config.BitsAPIToken,
		maxBodySize: maxBitsResponseBytes,
		logger:      log.Default(),
	}, nil
}

// FetchTripDetails performs one live fetch and extracts the response's data
// member. Every failure returns ErrLiveSourceUnavailable wrapped with a
// credential-free reason.
func (client *BitsTripDetailsClient) FetchTripDetails(ctx context.Context, lookup master.BitsLookup) (master.BitsResult, error) {
	requestURL, err := client.requestURL(lookup)
	if err != nil {
		return master.BitsResult{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureTransport, "build request")
	}
	httpRequest.Header.Set("Accept", "application/json")

	response, err := client.client.Do(httpRequest)
	if err != nil {
		// The error from Do embeds the request URL, which carries credentials,
		// so it is deliberately not wrapped.
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureTransport, "request failed")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		client.logger.Printf("Bits live fetch non-success: action=%s operator=%q host=%q status=%d",
			lookup.Action, lookup.OperatorCode, client.baseURL.Host, response.StatusCode)
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureStatus, "non-success status")
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, client.maxBodySize+1))
	if err != nil {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureTransport, "read body")
	}
	if int64(len(body)) > client.maxBodySize {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureBodyLimit, "body exceeds limit")
	}
	if len(body) == 0 {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureEmptyBody, "empty body")
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureInvalidJSON, "body is not a JSON object")
	}
	// A status member in the Bits body is deliberately ignored. The caller's
	// outcome derives from the HTTP status and the data member alone.
	raw, exists := envelope["data"]
	if !exists {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureNoDataMember, "no data member")
	}
	return client.dataResult(lookup, raw)
}

// dataResult classifies the data member. A null, empty array, or empty object
// is a successful fetch that found nothing.
func (client *BitsTripDetailsClient) dataResult(lookup master.BitsLookup, raw json.RawMessage) (master.BitsResult, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "null":
		return master.BitsResult{Empty: true}, nil
	case strings.HasPrefix(trimmed, "["):
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return master.BitsResult{}, client.fail(lookup, master.BitsFailureBadDataKind, "data array is invalid")
		}
		return master.BitsResult{
			Data:     append(json.RawMessage(nil), raw...),
			DataKind: master.BitsDataKindArray,
			Empty:    len(elements) == 0,
		}, nil
	case strings.HasPrefix(trimmed, "{"):
		var members map[string]json.RawMessage
		if err := json.Unmarshal(raw, &members); err != nil {
			return master.BitsResult{}, client.fail(lookup, master.BitsFailureBadDataKind, "data object is invalid")
		}
		return master.BitsResult{
			Data:     append(json.RawMessage(nil), raw...),
			DataKind: master.BitsDataKindObject,
			Empty:    len(members) == 0,
		}, nil
	default:
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureBadDataKind, "data is neither array nor object")
	}
}

// requestURL builds the action route with every dynamic segment escaped.
func (client *BitsTripDetailsClient) requestURL(lookup master.BitsLookup) (*url.URL, error) {
	segments := []string{"busservices", "api", "3.0", "json", lookup.OperatorCode, client.username, client.apiToken}
	switch lookup.Action {
	case master.BitsActionSearch:
		segments = append(segments, "search", lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	case master.BitsActionBusMap:
		segments = append(segments, "busmap", lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	default:
		return nil, client.fail(lookup, master.BitsFailureBadDataKind, "unsupported action")
	}

	requestURL := *client.baseURL
	requestURL.User = nil
	requestURL.RawQuery = ""
	requestURL.Fragment = ""

	// Build the escaped path first, then derive the decoded Path from it, so
	// that url.URL.RequestURI consistently emits the escaped form.
	escapedBase := strings.TrimRight(requestURL.EscapedPath(), "/")
	escaped := make([]string, len(segments))
	for index, segment := range segments {
		escaped[index] = url.PathEscape(segment)
	}
	requestURL.RawPath = escapedBase + "/" + strings.Join(escaped, "/")
	decoded, err := url.PathUnescape(requestURL.RawPath)
	if err != nil {
		return nil, client.fail(lookup, master.BitsFailureTransport, "build path")
	}
	requestURL.Path = decoded
	return &requestURL, nil
}

// fail logs a credential-free line and returns the wrapped sentinel.
func (client *BitsTripDetailsClient) fail(lookup master.BitsLookup, reason, detail string) error {
	client.logger.Printf("Bits live fetch failed: action=%s operator=%q host=%q reason=%s detail=%q",
		lookup.Action, lookup.OperatorCode, client.baseURL.Host, reason, detail)
	return fmt.Errorf("%w: %s", master.ErrLiveSourceUnavailable, reason)
}

var _ master.BitsTripDetailsFetcher = (*BitsTripDetailsClient)(nil)
