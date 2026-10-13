package master

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RateLimit defines the number of BITS requests allowed for one zone in a
// fixed window.
type RateLimit struct {
	Hits   int
	Window time.Duration
}

func (limit RateLimit) Enabled() bool {
	return limit.Hits > 0 && limit.Window > 0
}

// RateLimitPolicy resolves a default quota and optional exact zone-URL
// overrides. Master and Worker must use the same policy when sharing counters.
type RateLimitPolicy struct {
	Default   RateLimit
	Overrides map[string]RateLimit
}

func (policy RateLimitPolicy) Limit(zone string) RateLimit {
	if limit, ok := policy.Overrides[zone]; ok {
		return limit
	}
	return policy.Default
}

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

// ZoneRateLimiter enforces a BITS request quota for a resolved zone URL.
type ZoneRateLimiter interface {
	Acquire(ctx context.Context, zone string) (allowed bool, retryAfter time.Duration, err error)
}

var ErrZoneRateLimited = errors.New("BITS zone rate limit exceeded")

// ZoneRateLimitError carries the fixed-window reset time to the HTTP layer.
type ZoneRateLimitError struct {
	RetryAfter time.Duration
}

func (err *ZoneRateLimitError) Error() string {
	return fmt.Sprintf("%s; retry after %s", ErrZoneRateLimited, err.RetryAfter)
}

func (err *ZoneRateLimitError) Unwrap() error {
	return ErrZoneRateLimited
}
