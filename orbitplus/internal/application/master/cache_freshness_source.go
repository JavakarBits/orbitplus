package master

import (
	"context"
	"encoding/json"
	"errors"
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

// Reasons wrapped into ErrLiveSourceUnavailable. The set is closed so that
// error text can never carry upstream content.
const (
	BitsFailureTransport    = "transport"
	BitsFailureStatus       = "status"
	BitsFailureBodyLimit    = "body_limit"
	BitsFailureEmptyBody    = "empty_body"
	BitsFailureInvalidJSON  = "invalid_json"
	BitsFailureNoDataMember = "no_data_member"
	BitsFailureBadDataKind  = "bad_data_kind"
)

// BitsLookup identifies one live fetch. Action is BitsActionSearch or
// BitsActionBusMap; TripCode is used by busmap only.
type BitsLookup struct {
	Action       string
	OperatorCode string
	TripCode     string
	FromCode     string
	ToCode       string
	TravelDate   string
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
