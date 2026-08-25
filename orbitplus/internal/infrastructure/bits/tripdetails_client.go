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
	"sort"
	"strings"

	"orbitplusmaster/internal/application/master"
)

// maxBitsResponseBytes bounds how much of a Bits response is read, so a
// pathological upstream cannot exhaust master's memory.
const maxBitsResponseBytes int64 = 8 << 20

// BitsTripDetailsClient fetches a live TripDetails copy from Bits.
//
// The adapter holds neither an endpoint nor credentials. Both arrive on each
// BitsLookup, because operators live in different zones and authenticate as
// different principals, so one process-wide endpoint would query the wrong host
// for most operators. Credentials appear in exactly one place: the escaped path
// segments of an outbound request. They are never logged and never formatted
// into an error.
type BitsTripDetailsClient struct {
	client      *http.Client
	environment master.AppEnvironment
	maxBodySize int64
	logger      *log.Logger
}

// NewBitsTripDetailsClient constructs the adapter. The environment decides
// whether a plaintext zone endpoint is acceptable, so the check happens per
// request against the zone URL rather than once against configuration.
func NewBitsTripDetailsClient(httpClient *http.Client, environment master.AppEnvironment) (*BitsTripDetailsClient, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("Bits HTTP client is required")
	}
	return &BitsTripDetailsClient{
		client:      httpClient,
		environment: environment,
		maxBodySize: maxBitsResponseBytes,
		logger:      log.Default(),
	}, nil
}

// FetchTripDetails performs one live fetch and extracts the response's data
// member. Every failure returns ErrLiveSourceUnavailable wrapped with a
// credential-free reason.
func (client *BitsTripDetailsClient) FetchTripDetails(ctx context.Context, lookup master.BitsLookup) (master.BitsResult, error) {
	// Checked before the URL is built so a credential-less lookup never becomes
	// an outbound request with empty path segments, which Bits would answer
	// with an unrelated error.
	if !lookup.HasCredential() {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureMissingCredential, "credential absent")
	}
	if err := master.ValidateBitsURL(lookup.BaseURL, client.environment); err != nil {
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureZoneEndpoint, "zone endpoint rejected")
	}
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
			lookup.Action, lookup.OperatorCode, lookupHost(lookup), response.StatusCode)
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
	// A rejection is detected before the data member is read, because Bits
	// varies what it puts there when it refuses: 309A omits data entirely while
	// 318 sets it to a bare string. Reading data first turns one cause into two
	// unrelated-looking failures.
	//
	// This reverses an earlier decision to ignore the status member. Ignoring it
	// was only tenable while every non-success was assumed to arrive as a
	// non-2xx; Bits answers 200 with status 0 instead, so status is the only
	// reliable discriminator available.
	if code, description, rejected := bitsRejection(envelope); rejected {
		client.logger.Printf("Bits live fetch rejected: action=%s operator=%q host=%q errorCode=%s errorDesc=%s",
			lookup.Action, lookup.OperatorCode, lookupHost(lookup), code, description)
		return master.BitsResult{}, fmt.Errorf("%w: %s", master.ErrLiveSourceRejected, code)
	}

	raw, exists := envelope["data"]
	if !exists {
		// Absent data is the one failure a reason code cannot explain: the
		// fetch succeeded and the body parsed, so the cause is in content the
		// adapter otherwise discards. The envelope's shape is logged, never
		// returned, so the cause is recoverable without the body reaching a
		// caller.
		client.logger.Printf("Bits live fetch envelope carries no data member: action=%s operator=%q host=%q %s",
			lookup.Action, lookup.OperatorCode, lookupHost(lookup), describeEnvelope(envelope))
		return master.BitsResult{}, client.fail(lookup, master.BitsFailureNoDataMember, "no data member")
	}
	return client.dataResult(lookup, raw)
}

// maxDescribedMemberBytes bounds how much of a reported member is logged, so a
// large body cannot flood the log through the diagnostic path.
const maxDescribedMemberBytes = 200

// bitsRejection reports whether the body is Bits refusing the lookup, returning
// its error code and description for the log.
//
// A status member of exactly 0 is the signal. An absent or unreadable status is
// not treated as a rejection, so a body that does not follow the convention
// falls through to the existing data-member handling rather than being
// misreported as a refusal.
func bitsRejection(envelope map[string]json.RawMessage) (code string, description string, rejected bool) {
	raw, exists := envelope["status"]
	if !exists {
		return "", "", false
	}
	var status int
	if err := json.Unmarshal(raw, &status); err != nil || status != 0 {
		return "", "", false
	}
	return decodedMember(envelope, "errorCode"), decodedMember(envelope, "errorDesc"), true
}

// decodedMember renders one member for a log line. A JSON string is unquoted so
// the line stays readable; anything else is reported as its raw form.
func decodedMember(envelope map[string]json.RawMessage, name string) string {
	raw, exists := envelope[name]
	if !exists {
		return "absent"
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return truncateForLog(json.RawMessage(text))
	}
	return truncateForLog(raw)
}

// describeEnvelope summarises a Bits body for one log line.
//
// It reports the member names always, and the values of status and message only.
// Those two are Bits' own error reporting, so they name the cause; every other
// member is payload and is described by name alone.
func describeEnvelope(envelope map[string]json.RawMessage) string {
	members := make([]string, 0, len(envelope))
	for name := range envelope {
		members = append(members, name)
	}
	sort.Strings(members)

	described := fmt.Sprintf("members=%v", members)
	for _, name := range []string{"status", "message"} {
		if value, exists := envelope[name]; exists {
			described += fmt.Sprintf(" %s=%s", name, truncateForLog(value))
		}
	}
	return described
}

func truncateForLog(value json.RawMessage) string {
	trimmed := strings.TrimSpace(string(value))
	if len(trimmed) > maxDescribedMemberBytes {
		return trimmed[:maxDescribedMemberBytes] + "...(truncated)"
	}
	return trimmed
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
	segments := []string{"busservices", "api", "3.0", "json", lookup.OperatorCode, lookup.Username, lookup.APIToken}
	switch lookup.Action {
	case master.BitsActionSearch:
		segments = append(segments, "search", lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	case master.BitsActionBusMap:
		segments = append(segments, "busmap", lookup.TripCode, lookup.FromCode, lookup.ToCode, lookup.TravelDate)
	default:
		return nil, client.fail(lookup, master.BitsFailureBadDataKind, "unsupported action")
	}

	parsed, err := url.Parse(lookup.BaseURL)
	if err != nil {
		return nil, client.fail(lookup, master.BitsFailureZoneEndpoint, "zone endpoint is unparseable")
	}
	requestURL := *parsed
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
	decoded, unescapeErr := url.PathUnescape(requestURL.RawPath)
	if unescapeErr != nil {
		return nil, client.fail(lookup, master.BitsFailureTransport, "build path")
	}
	requestURL.Path = decoded
	return &requestURL, nil
}

// fail logs a credential-free line and returns the wrapped sentinel.
//
// The host is taken from the lookup's own endpoint, so the line names the zone
// that actually failed rather than a process-wide default.
func (client *BitsTripDetailsClient) fail(lookup master.BitsLookup, reason, detail string) error {
	client.logger.Printf("Bits live fetch failed: action=%s operator=%q host=%q reason=%s detail=%q",
		lookup.Action, lookup.OperatorCode, lookupHost(lookup), reason, detail)
	return fmt.Errorf("%w: %s", master.ErrLiveSourceUnavailable, reason)
}

// lookupHost reports the lookup's endpoint host for a log line, never its path,
// so no credential-bearing segment can be logged.
func lookupHost(lookup master.BitsLookup) string {
	parsed, err := url.Parse(lookup.BaseURL)
	if err != nil || parsed.Host == "" {
		return "unresolved"
	}
	return parsed.Host
}

var _ master.BitsTripDetailsFetcher = (*BitsTripDetailsClient)(nil)
