package master

import (
	"context"
	"log"
	"strings"
	"time"

	"orbitplusmaster/internal/domain"
)

// liveVerifier is the subset of CacheFreshnessVerifier the cold refresh needs:
// fetch the lookup live from BITS, compare, and repair the cache. Repair
// re-stamps metadata UpdatedAt and resets the Dragonfly TTLs, which is what
// returns a cold entry to Hot.
type liveVerifier interface {
	Verify(ctx context.Context, lookup BitsLookup, remoteAddress string) (VerifyResult, error)
}

// operatorZoneFinder resolves one operator's zone code so the cold refresh can
// derive the BITS base URL without the caller supplying a zone.
type operatorZoneFinder interface {
	FindOperator(ctx context.Context, operatorCode string) (domain.Operator, bool, error)
}

// operatorCredentialZoneReader reads one operator's stored credential so the
// cold refresh can use the operator-specific BITS zone URL that was resolved
// from Orbit, instead of a hardcoded zone-code map. It is optional: when nil,
// the refresher falls back to the zone-code map.
type operatorCredentialZoneReader interface {
	FindOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, bool, error)
}

// RefreshCredential carries the BITS credentials a cold refresh needs. They come
// from the read request's own path (operatorCode/username/apiToken), so no new
// credential source is introduced.
type RefreshCredential struct {
	Username string
	APIToken string
}

// Valid reports whether both credential halves are present.
func (credential RefreshCredential) Valid() bool {
	return strings.TrimSpace(credential.Username) != "" && strings.TrimSpace(credential.APIToken) != ""
}

// ColdCacheRefresher refreshes a stale (Cold) cached entry from BITS. It resolves
// the operator's zone to a BITS base URL, fetches live, and repairs the cache
// via the verifier. It is best-effort from the caller's point of view: if it
// cannot refresh (no credential, unknown zone, BITS down), the caller keeps
// serving the stale cache.
type ColdCacheRefresher struct {
	verifier    liveVerifier
	operators   operatorZoneFinder
	credentials operatorCredentialZoneReader
	logger      *log.Logger
}

// NewColdCacheRefresher constructs a cold refresher. A nil verifier or nil
// operator finder disables the refresh (RefreshCold reports refreshed=false),
// so the read path still serves whatever cache exists.
func NewColdCacheRefresher(verifier liveVerifier, operators operatorZoneFinder, logger *log.Logger) *ColdCacheRefresher {
	if logger == nil {
		logger = log.Default()
	}
	return &ColdCacheRefresher{verifier: verifier, operators: operators, logger: logger}
}

// WithCredentialZoneReader supplies the operator credential store so the cold
// refresh resolves the BITS base URL from the operator's stored zone URL
// (resolved from Orbit per operator) instead of the hardcoded zone-code map. A
// nil reader is ignored, leaving the zone-code map as the only source.
func (refresher *ColdCacheRefresher) WithCredentialZoneReader(reader operatorCredentialZoneReader) *ColdCacheRefresher {
	if reader != nil {
		refresher.credentials = reader
	}
	return refresher
}

// RefreshCold fetches the lookup live from BITS and repairs the cache. The
// action is BitsActionSearch or BitsActionBusMap. It returns true only when the
// live fetch succeeded and the cache was refreshed; on any failure it returns
// false so the caller serves the existing (stale) cache.
func (refresher *ColdCacheRefresher) RefreshCold(ctx context.Context, action string, lookup RouteLookup, credential RefreshCredential) bool {
	if refresher == nil || refresher.verifier == nil || refresher.operators == nil {
		return false
	}
	if !credential.Valid() {
		refresher.logger.Printf("cold cache refresh skipped: operator=%q reason=missing_credential", lookup.OperatorCode)
		return false
	}
	baseURL, ok := refresher.resolveZoneURL(ctx, lookup.OperatorCode)
	if !ok {
		return false
	}

	bitsLookup := BitsLookup{
		Action:       action,
		OperatorCode: lookup.OperatorCode,
		BaseURL:      baseURL,
		Username:     credential.Username,
		APIToken:     credential.APIToken,
		TripCode:     lookup.TripCode,
		FromCode:     lookup.FromCode,
		ToCode:       lookup.ToCode,
		TravelDate:   lookup.TripDate,
	}

	start := time.Now()
	refresher.logger.Printf("cold cache refresh started: action=%s operator=%q from=%q to=%q date=%q",
		action, lookup.OperatorCode, lookup.FromCode, lookup.ToCode, lookup.TripDate)
	if _, err := refresher.verifier.Verify(ctx, bitsLookup, "cold-cache-refresh"); err != nil {
		// Serving stale on failure is intentional: a cold entry still has usable
		// content, so an upstream problem must not turn a read into an error.
		refresher.logger.Printf("cold cache refresh failed; serving stale cache: action=%s operator=%q error=%v",
			action, lookup.OperatorCode, err)
		return false
	}
	refresher.logger.Printf("cold cache refresh completed: action=%s operator=%q duration=%s",
		action, lookup.OperatorCode, time.Since(start))
	return true
}

// resolveZoneURL turns an operator code into a BITS base URL. It prefers the
// operator's own stored credential zone URL (resolved from Orbit per operator),
// so the Master calls the same operator-specific BITS host the worker uses and
// nothing is hardcoded. Only when no stored zone URL is available does it fall
// back to the operator's zone code via the zone map. A missing operator or an
// unresolvable zone disables the refresh for this read.
func (refresher *ColdCacheRefresher) resolveZoneURL(ctx context.Context, operatorCode string) (string, bool) {
	// Prefer the operator-specific stored zone URL.
	if refresher.credentials != nil {
		credential, found, err := refresher.credentials.FindOperatorCredential(ctx, operatorCode)
		if err != nil {
			refresher.logger.Printf("cold cache refresh: stored zone url lookup failed, falling back to zone map: operator=%q error=%v", operatorCode, err)
		} else if found {
			if zoneURL := strings.TrimRight(strings.TrimSpace(credential.ZoneURL), "/"); zoneURL != "" {
				return zoneURL, true
			}
		}
	}

	// Fallback: resolve via the operator's registered zone code.
	operator, found, err := refresher.operators.FindOperator(ctx, operatorCode)
	if err != nil {
		refresher.logger.Printf("cold cache refresh skipped: operator=%q reason=operator_lookup_failed error=%v", operatorCode, err)
		return "", false
	}
	if !found {
		refresher.logger.Printf("cold cache refresh skipped: operator=%q reason=operator_not_found", operatorCode)
		return "", false
	}
	baseURL, resolved := ZoneBitsBaseURL(operator.ZoneCode)
	if !resolved {
		refresher.logger.Printf("cold cache refresh skipped: operator=%q zone=%q reason=zone_url_unresolved", operatorCode, operator.ZoneCode)
		return "", false
	}
	return baseURL, true
}
