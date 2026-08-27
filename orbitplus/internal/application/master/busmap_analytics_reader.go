package master

import (
	"context"
	"errors"
	"sort"

	"orbitplusmaster/internal/domain"
)

// busmapAnalyticsReportLimit bounds the sample the report loads, matching the
// queue metrics report so one slow read cannot pull an unbounded result set.
const busmapAnalyticsReportLimit = 100

// BusmapAnalyticsReader reads a bounded sample of recorded cache differences.
type BusmapAnalyticsReader interface {
	List(ctx context.Context, limit int) ([]domain.RecordedDifference, error)
}

// BusmapAnalyticsService provides recorded cache-versus-Bits differences for
// the report UI.
type BusmapAnalyticsService struct {
	reader BusmapAnalyticsReader
}

// NewBusmapAnalyticsService constructs the report reader. A nil reader leaves
// the report unavailable rather than failing the whole service.
func NewBusmapAnalyticsService(reader BusmapAnalyticsReader) *BusmapAnalyticsService {
	return &BusmapAnalyticsService{reader: reader}
}

// List returns a bounded sample, newest detection first within that sample. The
// scan returns token-order rows, so the sort is what makes the report read as
// most-recent, exactly as the queue metrics report does.
func (service *BusmapAnalyticsService) List(ctx context.Context) ([]domain.RecordedDifference, error) {
	if service == nil || service.reader == nil {
		return nil, errors.New("busmap data analytics reporting is not configured")
	}
	records, err := service.reader.List(ctx, busmapAnalyticsReportLimit)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].DetectedAt.After(records[right].DetectedAt)
	})
	return records, nil
}
