// Package dragonfly implements the worker's distributed per-zone rate limiter
// backed by the project's existing Dragonfly (Redis-compatible) cache.
package dragonfly

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"orbitplusworker/internal/application/worker"
)

// Config contains the Dragonfly connection settings. It mirrors the master
// service's Dragonfly configuration so both services share one cache cluster.
type Config struct {
	Address     string
	Password    string
	Database    int
	DialTimeout time.Duration
}

// rateLimitKeyPrefix namespaces the per-zone counter keys so they cannot
// collide with the master service's cached TripDetails documents.
const rateLimitKeyPrefix = "bits:ratelimit:zone:"

// rateLimitScript increments the zone's hit counter and, on the first hit of a
// window, sets the window expiry. Doing both in one script makes the count and
// its expiry atomic, so a crash cannot leave a counter without a TTL and
// concurrent workers cannot together exceed the quota. It returns the current
// count and the window's remaining milliseconds.
var rateLimitScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
	return {count, ARGV[1]}
end
return {count, redis.call('PTTL', KEYS[1])}
`)

// ZoneRateLimiter is a distributed per-zone fixed-window limiter. Each Acquire
// counts one hit against the zone's quota within the current window; when the
// count exceeds the quota the zone is blocked until the window resets. Because
// the count is maintained in Dragonfly, the quota holds across all worker
// goroutines and process instances.
type ZoneRateLimiter struct {
	client *redis.Client
	policy worker.RateLimitPolicy
}

// NewZoneRateLimiter connects to Dragonfly and verifies reachability up front so
// a misconfigured cache is a visible startup failure rather than a silent
// bypass of the rate limit.
func NewZoneRateLimiter(ctx context.Context, config Config, policy worker.RateLimitPolicy) (*ZoneRateLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr: config.Address, Password: config.Password, DB: config.Database, DialTimeout: config.DialTimeout,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Dragonfly: %w", err)
	}
	return &ZoneRateLimiter{client: client, policy: policy}, nil
}

// Acquire records one hit for zone and reports whether it stayed within the
// zone's quota. It is safe across goroutines and process instances because the
// increment and expiry run as one atomic script.
func (limiter *ZoneRateLimiter) Acquire(ctx context.Context, zone string) (bool, time.Duration, error) {
	limit := limiter.policy.Limit(zone)
	if !limit.Enabled() {
		return true, 0, nil
	}
	key := rateLimitKeyPrefix + zone
	reply, err := rateLimitScript.Run(ctx, limiter.client, []string{key}, limit.Window.Milliseconds()).Int64Slice()
	if err != nil {
		return false, 0, fmt.Errorf("evaluate zone rate-limit window: %w", err)
	}
	if len(reply) != 2 {
		return false, 0, fmt.Errorf("unexpected zone rate-limit reply length %d", len(reply))
	}
	count, remainingMillis := reply[0], reply[1]
	if count <= int64(limit.Hits) {
		return true, 0, nil
	}
	// Quota exhausted for this window. Report the reset time so the caller can
	// pause before requeuing rather than spinning.
	retryAfter := time.Duration(remainingMillis) * time.Millisecond
	if retryAfter < 0 {
		retryAfter = 0
	}
	return false, retryAfter, nil
}

// Close releases the underlying Dragonfly connection.
func (limiter *ZoneRateLimiter) Close() error { return limiter.client.Close() }

var _ worker.ZoneRateLimiter = (*ZoneRateLimiter)(nil)
