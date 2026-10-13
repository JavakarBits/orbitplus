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
	verifier  liveVerifier
	operators operatorZoneFinder
	logger    *log.Logger
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

// resolveZoneURL turns an operator code into a BITS base URL via its stored zone
// code. A missing operator or unknown zone disables the refresh for this read.
func (refresher *ColdCacheRefresher) resolveZoneURL(ctx context.Context, operatorCode string) (string, bool) {
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
