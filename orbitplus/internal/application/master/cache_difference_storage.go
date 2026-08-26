package master

import (
	"context"
	"fmt"
	"time"

	"orbitplusmaster/internal/domain"
)

// maxStoredPaths caps how many difference paths one row keeps. The untruncated
// total is still reported in DifferenceCount, so a wholly different document
// produces a bounded row rather than an unbounded one.
const maxStoredPaths = 200

// maxStoredValueChars bounds each rendered side of a difference. A value
// mismatch is usually a scalar, but a MISSING entry's value is a whole entry,
// so without a cap one difference could carry an entire document into the row.
const maxStoredValueChars = 120

// formatDifferencePath renders one difference as "KIND path: cache → bits", so
// the row shows what changed rather than only where. The kind decides which
// sides are meaningful: a value mismatch has both, a missing entry has one.
func formatDifferencePath(difference domain.DifferenceEntry) string {
	head := string(difference.Kind) + " " + difference.Path
	switch difference.Kind {
	case domain.DifferenceMissingInCache:
		return head + ": " + truncateValue(difference.BitsValue)
	case domain.DifferenceMissingInBits:
		return head + ": " + truncateValue(difference.CacheValue)
	default:
		return head + ": " + truncateValue(difference.CacheValue) + " \u2192 " + truncateValue(difference.BitsValue)
	}
}

// truncateValue bounds one rendered value and marks where it was cut, so a long
// value is still recognisable without bloating the row.
func truncateValue(value string) string {
	if value == "" {
		return "∅"
	}
	runes := []rune(value)
	if len(runes) <= maxStoredValueChars {
		return value
	}
	return string(runes[:maxStoredValueChars]) + "…"
}

// CacheDifferenceWriter persists one non-matching verification result.
type CacheDifferenceWriter interface {
	SaveDifference(ctx context.Context, record domain.RecordedDifference) error
}

// buildRecordedDifference shapes one summary row.
func buildRecordedDifference(
	lookup BitsLookup,
	outcome domain.VerificationOutcome,
	differences []domain.DifferenceEntry,
	total int,
	repaired bool,
	detectedAt time.Time,
) domain.RecordedDifference {
	paths := make([]string, 0, len(differences))
	for _, difference := range differences {
		if len(paths) >= maxStoredPaths {
			break
		}
		paths = append(paths, formatDifferencePath(difference))
	}

	record := domain.RecordedDifference{
		OperatorCode:        lookup.OperatorCode,
		DetectedOn:          detectedAt.UTC().Format("2006-01-02"),
		DetectedAt:          detectedAt.UTC(),
		ActionType:          lookup.Action,
		FromCode:            lookup.FromCode,
		ToCode:              lookup.ToCode,
		TripDate:            lookup.TravelDate,
		TripCode:            lookup.TripCode,
		VerificationOutcome: outcome,
		DifferenceCount:     total,
		DifferencePaths:     paths,
		CacheRepaired:       repaired,
	}
	// Attribute the row to the first differing entry when one exists, so an
	// operator can jump straight to the trip that disagreed.
	if len(differences) > 0 {
		if differences[0].Identity.TripCode != "" {
			record.TripCode = differences[0].Identity.TripCode
		}
		record.TripStageCode = differences[0].Identity.TripStageCode
	}
	return record
}

// describeOutcome summarises a row for a log line without carrying payload.
func describeOutcome(record domain.RecordedDifference) string {
	return fmt.Sprintf("outcome=%s differences=%d paths=%d repaired=%t",
		record.VerificationOutcome, record.DifferenceCount, len(record.DifferencePaths), record.CacheRepaired)
}
