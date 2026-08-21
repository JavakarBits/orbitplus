package domain

import "time"

// DifferenceKind classifies one recorded disagreement between the cached copy
// and the live Bits copy.
type DifferenceKind string

const (
	// DifferenceValueMismatch is a value that differs at one JSON path.
	DifferenceValueMismatch DifferenceKind = "VALUE_MISMATCH"
	// DifferenceMissingInCache is an entry Bits returned that the cache lacks.
	DifferenceMissingInCache DifferenceKind = "MISSING_IN_CACHE"
	// DifferenceMissingInBits is a cached entry Bits no longer returns.
	DifferenceMissingInBits DifferenceKind = "MISSING_IN_BITS"
)

// VerificationOutcome is the result of one live verification.
type VerificationOutcome string

const (
	// OutcomeMatch means the two copies are equal once canonicalised. No row
	// is stored, so the table grows with problems rather than with traffic.
	OutcomeMatch VerificationOutcome = "MATCH"
	// OutcomeDifferent means at least one difference was found.
	OutcomeDifferent VerificationOutcome = "DIFFERENT"
	// OutcomeCacheMissing means Bits returned data the cache holds nothing for.
	OutcomeCacheMissing VerificationOutcome = "CACHE_MISSING"
	// OutcomeSourceUnavailable means one side could not be obtained.
	OutcomeSourceUnavailable VerificationOutcome = "SOURCE_UNAVAILABLE"
)

// EntryIdentity pairs a cached entry with a Bits entry. Two entries describe
// the same thing when all three components match.
type EntryIdentity struct {
	OperatorCode  string
	TripCode      string
	TripStageCode string
}

// DifferenceEntry is one disagreement at one JSON path inside one entry.
//
// CacheValue and BitsValue hold canonical JSON renderings rather than Go
// values, so a DifferenceEntry is safe to log and to persist as-is.
type DifferenceEntry struct {
	Identity   EntryIdentity
	Path       string
	Kind       DifferenceKind
	CacheValue string
	BitsValue  string
}

// RecordedDifference is one row of the cache_freshness_difference table.
//
// This is deliberately a summary, not an archive. Full before-and-after payloads
// are not stored: they are large, they duplicate what Bits can be asked for
// again, and the cache is repaired immediately anyway, so a stored snapshot
// would describe a state that no longer exists. The paths are what make a
// difference actionable.
type RecordedDifference struct {
	DifferenceID        string
	OperatorCode        string
	DetectedOn          string // UTC calendar date, YYYY-MM-DD
	DetectedAt          time.Time
	ActionType          string
	TripCode            string
	TripStageCode       string
	FromCode            string
	ToCode              string
	TripDate            string
	VerificationOutcome VerificationOutcome
	// DifferenceCount is the untruncated total, which can exceed the number of
	// persisted paths.
	DifferenceCount int
	// DifferencePaths names where the copies disagreed, for example
	// "VALUE_MISMATCH bus.seatLayoutList[3].fare".
	DifferencePaths []string
	// CacheRepaired reports whether the cached copy was overwritten with the
	// live copy after the difference was found.
	CacheRepaired bool
}
