package dragonfly

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"orbitplusmaster/internal/application/master"
)

// Keep this key and Lua algorithm identical to orbitplusworker so Master live
// requests and Worker background requests share one per-zone quota.
const rateLimitKeyPrefix = "bits:ratelimit:zone:"

var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
	return {count, ARGV[1]}
end
return {count, redis.call('PTTL', KEYS[1])}
`)

// ZoneRateLimiter borrows the TripDetails cache Redis client. The cache
// repository remains the sole owner and closes it during Master shutdown.
type ZoneRateLimiter struct {
	client *redis.Client
	policy master.RateLimitPolicy
}

func NewZoneRateLimiter(cache *TripDetailsCacheRepository, policy master.RateLimitPolicy) (*ZoneRateLimiter, error) {
	if cache == nil || cache.client == nil {
		return nil, fmt.Errorf("Dragonfly cache is required for BITS rate limiting")
	}
	return &ZoneRateLimiter{client: cache.client, policy: policy}, nil
}

func (limiter *ZoneRateLimiter) Acquire(ctx context.Context, zone string) (bool, time.Duration, error) {
	limit := limiter.policy.Limit(zone)
	if !limit.Enabled() {
		return true, 0, nil
	}
	reply, err := rateLimitScript.Run(ctx, limiter.client, []string{rateLimitKeyPrefix + zone}, limit.Window.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("evaluate zone rate-limit window: %w", err)
	}
	if len(reply) != 2 {
		return false, 0, fmt.Errorf("unexpected zone rate-limit reply length %d", len(reply))
	}
	if reply[0] <= int64(limit.Hits) {
		return true, 0, nil
	}
	retryAfter := time.Duration(reply[1]) * time.Millisecond
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter, nil
}

var _ master.ZoneRateLimiter = (*ZoneRateLimiter)(nil)
