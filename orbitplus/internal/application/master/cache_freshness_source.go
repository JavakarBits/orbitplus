package master

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Bits actions master can fetch live. Unlike the Worker, master has no
// searchbusmap action: the read routes it serves are search and busmap only.
const (
	BitsActionSearch = "SEARCH"
	BitsActionBusMap = "BUSMAP"
)

// ErrLiveSourceUnavailable reports that the live Bits copy could not be
// obtained. Every adapter failure mode collapses to this error wrapped with a
// credential-free reason drawn from a closed set, so no upstream detail and no
// credential can reach a caller through an error string.
var ErrLiveSourceUnavailable = errors.New("live source unavailable")

// ErrLiveSourceRejected reports that Bits answered normally but refused the
// lookup, signalled by a status member of 0 alongside an errorCode.
//
// This is deliberately separate from ErrLiveSourceUnavailable. Bits is reachable
// and healthy here, so reporting it as an unavailable source sends an operator
// to investigate an outage that is not happening, and invites the caller to
// retry a request that cannot ever succeed. Observed codes include 309A for an
// expired travel date and 318 for a route the operator does not serve.
var ErrLiveSourceRejected = errors.New("live source rejected the lookup")

// Reasons wrapped into ErrLiveSourceUnavailable. The set is closed so that
// error text can never carry upstream content.
const (
	BitsFailureTransport         = "transport"
	BitsFailureStatus            = "status"
	BitsFailureBodyLimit         = "body_limit"
	BitsFailureEmptyBody         = "empty_body"
	BitsFailureInvalidJSON       = "invalid_json"
	BitsFailureNoDataMember      = "no_data_member"
	BitsFailureBadDataKind       = "bad_data_kind"
	BitsFailureMissingCredential = "missing_credential"
)

// BitsLookup identifies one live fetch. Action is BitsActionSearch or
// BitsActionBusMap; TripCode is used by busmap only.
//
// Username and APIToken are the credentials the caller supplied on the read
// route, carried per lookup rather than held on the adapter because each
// request may authenticate to Bits as a different operator. They are used to
// build one outbound path and nothing else: no log line, error, response body,
// difference row, or cache key may ever contain them.
type BitsLookup struct {
	Action       string
	OperatorCode string
	Username     string
	APIToken     string
	TripCode     string
	FromCode     string
	ToCode       string
	TravelDate   string
}

// HasCredential reports whether the lookup carries both halves of a credential.
func (lookup BitsLookup) HasCredential() bool {
	return strings.TrimSpace(lookup.Username) != "" && strings.TrimSpace(lookup.APIToken) != ""
}

// BitsResult carries the extracted data member of a successful Bits response.
//
// Data holds the member's original bytes so it can be served through unchanged.
// Empty reports a data member of null, an empty array, or an empty object,
// which is a successful fetch that found nothing rather than a failure.
type BitsResult struct {
	Data     json.RawMessage
	DataKind string
	Empty    bool
}

// Data kinds reported by BitsResult.
const (
	BitsDataKindArray  = "array"
	BitsDataKindObject = "object"
)

// BitsTripDetailsFetcher is the application-owned live source capability.
// Infrastructure supplies the implementation, so no application code imports
// net/http.
type BitsTripDetailsFetcher interface {
	FetchTripDetails(ctx context.Context, lookup BitsLookup) (BitsResult, error)
}
