package master

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"orbitplusmaster/internal/domain"
)

// ErrCacheFreshnessNotConfigured means no metadata reader is available to count.
var ErrCacheFreshnessNotConfigured = errors.New("cache freshness count is not configured")

// CacheFreshnessMetadataReader reads route metadata used to count cache entries
// by freshness tier. *cassandra.TripDetailsMetadataRepository implements it.
type CacheFreshnessMetadataReader interface {
	ListRouteMetadata(context.Context) ([]domain.TripDetailsStageMetadata, error)
}

// CacheFreshnessCounts is a point-in-time snapshot of how many unique cached
// trip stages fall into each freshness tier, classified by age exactly as the
// read path classifies them (age = now - UpdatedAt, using hotMaxAge/warmMaxAge).
type CacheFreshnessCounts struct {
	Hot   int `json:"hot"`
	Warm  int `json:"warm"`
	Cold  int `json:"cold"`
	Total int `json:"total"`
	// HotMaxAge and WarmMaxAge are the thresholds used, surfaced so the UI can
	// show the boundaries that define each tier.
	HotMaxAge  string    `json:"hotMaxAge"`
	WarmMaxAge string    `json:"warmMaxAge"`
	ObservedAt time.Time `json:"observedAt"`
}

// CacheFreshnessCountService tallies cached trip stages by freshness tier
// (Hot/Warm/Cold) using the same age thresholds the read path uses. It counts
// UNIQUE stages (operator, tripDate, tripCode, tripStageCode), using the latest
// UpdatedAt per stage, so duplicate metadata rows do not inflate the totals.
type CacheFreshnessCountService struct {
	metadata CacheFreshnessMetadataReader
	now      func() time.Time
}

// NewCacheFreshnessCountService constructs the service. A nil reader disables
// the feature, so Counts reports ErrCacheFreshnessNotConfigured.
func NewCacheFreshnessCountService(metadata CacheFreshnessMetadataReader) *CacheFreshnessCountService {
	return &CacheFreshnessCountService{metadata: metadata, now: time.Now}
}

// Counts returns the current Hot/Warm/Cold/Total snapshot.
func (service *CacheFreshnessCountService) Counts(ctx context.Context) (CacheFreshnessCounts, error) {
	if service == nil || service.metadata == nil {
		return CacheFreshnessCounts{}, ErrCacheFreshnessNotConfigured
	}
	now := service.now().UTC()
	rows, err := service.metadata.ListRouteMetadata(ctx)
	if err != nil {
		return CacheFreshnessCounts{}, err
	}

	// Collapse duplicate metadata rows to one entry per unique stage, keeping
	// the newest UpdatedAt so the tier reflects the freshest known state.
	latest := make(map[cacheFreshnessKey]time.Time, len(rows))
	for _, row := range rows {
		key := cacheFreshnessKey{
			operatorCode:  row.OperatorCode,
			tripDate:      row.TripDate,
			tripCode:      row.TripCode,
			tripStageCode: row.TripStageCode,
		}
		if updatedAt, exists := latest[key]; !exists || row.UpdatedAt.After(updatedAt) {
			latest[key] = row.UpdatedAt
		}
	}

	counts := CacheFreshnessCounts{
		Total:      len(latest),
		HotMaxAge:  hotMaxAge.String(),
		WarmMaxAge: warmMaxAge.String(),
		ObservedAt: now,
	}
	for _, updatedAt := range latest {
		switch classifyFreshness(updatedAt, now) {
		case TierHot:
			counts.Hot++
		case TierWarm:
			counts.Warm++
		default:
			counts.Cold++
		}
	}
	return counts, nil
}

type cacheFreshnessKey struct {
	operatorCode  string
	tripDate      string
	tripCode      string
	tripStageCode string
}

// CacheFreshnessEntry is one cached trip stage in a given freshness tier, with
// enough identity to find it and its age so the UI can show why it's in that
// tier.
type CacheFreshnessEntry struct {
	OperatorCode  string    `json:"operatorCode"`
	TripCode      string    `json:"tripCode"`
	TripStageCode string    `json:"tripStageCode"`
	TripDate      string    `json:"tripDate"`
	FromStation   string    `json:"fromStation"`
	ToStation     string    `json:"toStation"`
	UpdatedAt     time.Time `json:"updatedAt"`
	AgeSeconds    int64     `json:"ageSeconds"`
	Tier          string    `json:"tier"`
}

// ParseFreshnessTier maps a "hot"/"warm"/"cold" string to a FreshnessTier.
func ParseFreshnessTier(value string) (FreshnessTier, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hot":
		return TierHot, true
	case "warm":
		return TierWarm, true
	case "cold":
		return TierCold, true
	default:
		return TierHot, false
	}
}

// Entries returns the unique trip stages currently in the requested freshness
// tier, newest first. Like Counts, it collapses duplicate rows to the latest
// UpdatedAt per stage before classifying, so the entry list matches the counts.
func (service *CacheFreshnessCountService) Entries(ctx context.Context, tier FreshnessTier) ([]CacheFreshnessEntry, error) {
	if service == nil || service.metadata == nil {
		return nil, ErrCacheFreshnessNotConfigured
	}
	now := service.now().UTC()
	rows, err := service.metadata.ListRouteMetadata(ctx)
	if err != nil {
		return nil, err
	}

	// Keep the newest row per unique stage so stations/age reflect the freshest
	// known state, matching how Counts dedupes.
	latest := make(map[cacheFreshnessKey]domain.TripDetailsStageMetadata, len(rows))
	for _, row := range rows {
		key := cacheFreshnessKey{
			operatorCode:  row.OperatorCode,
			tripDate:      row.TripDate,
			tripCode:      row.TripCode,
			tripStageCode: row.TripStageCode,
		}
		if existing, exists := latest[key]; !exists || row.UpdatedAt.After(existing.UpdatedAt) {
			latest[key] = row
		}
	}

	entries := make([]CacheFreshnessEntry, 0)
	for _, row := range latest {
		if classifyFreshness(row.UpdatedAt, now) != tier {
			continue
		}
		entries = append(entries, CacheFreshnessEntry{
			OperatorCode:  row.OperatorCode,
			TripCode:      row.TripCode,
			TripStageCode: row.TripStageCode,
			TripDate:      row.TripDate,
			FromStation:   row.FromStationCode,
			ToStation:     row.ToStationCode,
			UpdatedAt:     row.UpdatedAt,
			AgeSeconds:    int64(now.Sub(row.UpdatedAt).Seconds()),
			Tier:          tier.String(),
		})
	}
	// Newest first (smallest age first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})
	return entries, nil
}
