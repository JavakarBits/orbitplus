package worker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimit expresses a per-zone quota as a number of allowed BITS hits within a
// fixed time window (for example 10 hits per 1 minute). A zero Hits or Window
// disables limiting for the zone.
type RateLimit struct {
	Hits   int
	Window time.Duration
}

// Enabled reports whether the quota is active.
func (limit RateLimit) Enabled() bool { return limit.Hits > 0 && limit.Window > 0 }

// String renders the quota as "hits/window" for logs.
func (limit RateLimit) String() string {
	if !limit.Enabled() {
		return "disabled"
	}
	return fmt.Sprintf("%d/%s", limit.Hits, limit.Window)
}

// ZoneRateLimiter enforces a per-zone quota of BITS hits per window. A "zone" is
// identified by the request's Bits endpoint (the message ZoneURL), which is
// exactly the host every BITS request for that zone targets.
//
// Implementations must be safe for concurrent use by multiple worker goroutines,
// and the distributed implementation must be safe across process instances so
// concurrent workers cannot together exceed a zone's quota.
type ZoneRateLimiter interface {
	// Acquire records one BITS hit for zone and reports whether it is within the
	// zone's quota. When allowed is true the caller may call BITS now. When
	// allowed is false the zone has used its full quota for the current window
	// and retryAfter is the time until the window resets.
	Acquire(ctx context.Context, zone string) (allowed bool, retryAfter time.Duration, err error)
}

// RateLimitPolicy resolves the quota for a zone. Default applies to any zone
// without an explicit override. A disabled quota turns limiting off for the zone.
type RateLimitPolicy struct {
	Default   RateLimit
	Overrides map[string]RateLimit
}

// Limit returns the quota that applies to zone.
func (policy RateLimitPolicy) Limit(zone string) RateLimit {
	if limit, ok := policy.Overrides[zone]; ok {
		return limit
	}
	return policy.Default
}

// Enabled reports whether any zone has an active quota. When false the
// composition root installs the no-op limiter.
func (policy RateLimitPolicy) Enabled() bool {
	if policy.Default.Enabled() {
		return true
	}
	for _, limit := range policy.Overrides {
		if limit.Enabled() {
			return true
		}
	}
	return false
}

// AllowAllRateLimiter is the no-op limiter installed when rate limiting is
// disabled. It never blocks a zone.
type AllowAllRateLimiter struct{}

func (AllowAllRateLimiter) Acquire(context.Context, string) (bool, time.Duration, error) {
	return true, 0, nil
}

// inMemoryWindow tracks the start and hit count of one zone's current window.
type inMemoryWindow struct {
	start time.Time
	count int
}

// InMemoryZoneRateLimiter is a process-local fixed-window limiter for
// single-instance deployments. It is safe for concurrent goroutines but does
// not coordinate across process instances; use the distributed limiter when
// multiple worker instances run against the same BITS zones.
type InMemoryZoneRateLimiter struct {
	policy  RateLimitPolicy
	now     func() time.Time
	mu      sync.Mutex
	windows map[string]*inMemoryWindow
}

func NewInMemoryZoneRateLimiter(policy RateLimitPolicy) *InMemoryZoneRateLimiter {
	return &InMemoryZoneRateLimiter{policy: policy, now: time.Now, windows: make(map[string]*inMemoryWindow)}
}

func (limiter *InMemoryZoneRateLimiter) Acquire(_ context.Context, zone string) (bool, time.Duration, error) {
	limit := limiter.policy.Limit(zone)
	if !limit.Enabled() {
		return true, 0, nil
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.now()
	window, ok := limiter.windows[zone]
	if !ok || now.Sub(window.start) >= limit.Window {
		// Start a fresh window; this hit is the first one in it.
		limiter.windows[zone] = &inMemoryWindow{start: now, count: 1}
		return true, 0, nil
	}
	if window.count >= limit.Hits {
		return false, window.start.Add(limit.Window).Sub(now), nil
	}
	window.count++
	return true, 0, nil
}

var _ ZoneRateLimiter = AllowAllRateLimiter{}
var _ ZoneRateLimiter = (*InMemoryZoneRateLimiter)(nil)
