package master

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Cache tier retention windows. Hot (trip/stage) is small and read on every
// search; Warm (busmap) is large and read on demand, so it is kept for less
// time. Both are overridable via env vars.
//
// Trip/stage are NOT demoted by travel date: Orionmax proactively pushes
// far-future trips precisely so they can be read later, and the Cassandra route
// metadata has no TTL. If the content TTL were shorter than the metadata, a
// search would find metadata but miss the expired content and return not-found.
const (
	defaultHotCacheTTL  = 24 * time.Hour
	defaultWarmCacheTTL = 12 * time.Hour
)

var (
	hotCacheTTL  = durationFromEnv("CACHE_HOT_TTL", defaultHotCacheTTL)
	warmCacheTTL = durationFromEnv("CACHE_WARM_TTL", defaultWarmCacheTTL)
)

// Freshness tiers classify a cached entry by AGE, independently of the Dragonfly
// TTLs above. Age is measured as now - metadata.UpdatedAt. The TTLs decide when a
// key is evicted; these thresholds decide when a still-present entry is too old
// to trust and must be refreshed from BITS.
//
//	age <= hotMaxAge              -> Hot  (serve cache)
//	hotMaxAge < age <= warmMaxAge -> Warm (serve cache)
//	age > warmMaxAge              -> Cold (refresh from BITS, then serve)
const (
	defaultHotMaxAge  = 4 * time.Hour
	defaultWarmMaxAge = 6 * time.Hour
)

var (
	hotMaxAge  = durationFromEnv("CACHE_HOT_MAX_AGE", defaultHotMaxAge)
	warmMaxAge = durationFromEnv("CACHE_WARM_MAX_AGE", defaultWarmMaxAge)
)

// FreshnessTier is the age classification of a cached entry.
type FreshnessTier int

const (
	// TierHot is fresh data within hotMaxAge; served directly from cache.
	TierHot FreshnessTier = iota
	// TierWarm is acceptable data within warmMaxAge; still served from cache.
	TierWarm
	// TierCold is stale data older than warmMaxAge; refreshed from BITS before serving.
	TierCold
)

func (tier FreshnessTier) String() string {
	switch tier {
	case TierHot:
		return "HOT"
	case TierWarm:
		return "WARM"
	default:
		return "COLD"
	}
}

// classifyFreshness buckets an entry by how old its last update is. A zero
// updatedAt (missing timestamp) is treated as Cold so an unknown-age entry is
// refreshed rather than trusted.
func classifyFreshness(updatedAt, now time.Time) FreshnessTier {
	if updatedAt.IsZero() {
		return TierCold
	}
	age := now.Sub(updatedAt)
	switch {
	case age <= hotMaxAge:
		return TierHot
	case age <= warmMaxAge:
		return TierWarm
	default:
		return TierCold
	}
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

// TripDetailsCacheRepository stores JSON content in Dragonfly without exposing
// cache-client details to the application layer.
type TripDetailsCacheRepository interface {
	SetJSONWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// BuildTripKey returns the Dragonfly key for common Search Trip content.
func BuildTripKey(operatorCode, tripCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode); err != nil {
		return "", err
	}
	return "trip:" + operatorCode + ":" + tripCode, nil
}

// BuildStageKey returns the Dragonfly key for Search Stage content.
func BuildStageKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "stage:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

// BuildBusMapKey returns the Dragonfly key for a complete BUSMAP response.
func BuildBusMapKey(operatorCode, tripCode, tripStageCode string) (string, error) {
	if err := requireKeyComponents(operatorCode, tripCode, tripStageCode); err != nil {
		return "", err
	}
	return "busmap:" + operatorCode + ":" + tripCode + ":" + tripStageCode, nil
}

func requireKeyComponents(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.Contains(value, ":") {
			return errMissingStorageIdentifiers
		}
	}
	return nil
}

func (persistence *TripDetailsStorage) storeSearchCache(ctx context.Context, operatorCode, tripCode, tripStageCode string, entry map[string]any, index int) error {
	tripKey, err := BuildTripKey(operatorCode, tripCode)
	if err != nil {
		return err
	}
	stageKey, err := BuildStageKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails Dragonfly SEARCH keys entry=%d trip=%q stage=%q ttl=%s", index, tripKey, stageKey, hotCacheTTL)
	tripJSON, err := json.Marshal(commonTripProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal common Trip JSON: %w", err)
	}
	stageJSON, err := json.Marshal(stageProjection(entry))
	if err != nil {
		return fmt.Errorf("marshal Stage JSON: %w", err)
	}
	if err := persistence.cache.SetJSONWithTTL(ctx, tripKey, tripJSON, hotCacheTTL); err != nil {
		return fmt.Errorf("store common Trip content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=trip key=%q", index, tripKey)
	if err := persistence.cache.SetJSONWithTTL(ctx, stageKey, stageJSON, hotCacheTTL); err != nil {
		return fmt.Errorf("store Stage content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=stage key=%q", index, stageKey)
	return nil
}

func (persistence *TripDetailsStorage) storeBusMapCache(ctx context.Context, operatorCode, tripCode, tripStageCode string, entry map[string]any, index int) error {
	busMapKey, err := BuildBusMapKey(operatorCode, tripCode, tripStageCode)
	if err != nil {
		return err
	}
	persistence.logger.Printf("TripDetails BUSMAP persistence entry=%d identifiers operatorCode=%q tripCode=%q tripStageCode=%q", index, operatorCode, tripCode, tripStageCode)
	busMapJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal BUSMAP JSON: %w", err)
	}
	if err := persistence.cache.SetJSONWithTTL(ctx, busMapKey, busMapJSON, warmCacheTTL); err != nil {
		return fmt.Errorf("store BUSMAP content: %w", err)
	}
	persistence.logger.Printf("TripDetails Dragonfly write succeeded entry=%d cache=busmap key=%q", index, busMapKey)
	return nil
}

var searchStageFields = map[string]struct{}{
	"tripStageCode": {}, "fromStation": {}, "toStation": {}, "stationPoint": {}, "stageFare": {},
}

func commonTripProjection(entry map[string]any) map[string]any {
	common := make(map[string]any, len(entry))
	for key, value := range entry {
		if _, isStageField := searchStageFields[key]; isStageField {
			continue
		}
		if key == "bus" {
			if bus, ok := value.(map[string]any); ok {
				common[key] = copyWithoutSeatLayoutList(bus)
				continue
			}
		}
		common[key] = value
	}
	return common
}

func stageProjection(entry map[string]any) map[string]any {
	stage := make(map[string]any, len(searchStageFields))
	for key := range searchStageFields {
		if value, exists := entry[key]; exists {
			stage[key] = value
		}
	}
	return stage
}

func copyWithoutSeatLayoutList(bus map[string]any) map[string]any {
	copy := make(map[string]any, len(bus))
	for key, value := range bus {
		if key != "seatLayoutList" {
			copy[key] = value
		}
	}
	return copy
}
