package worker

import (
	"context"
	"testing"
	"time"
)

// TestInMemoryLimiterHitsPerWindow verifies the "N hits per window" quota: the
// first N hits are allowed, the next is blocked, and the window resets after the
// configured duration.
func TestInMemoryLimiterHitsPerWindow(t *testing.T) {
	zone := "http://app.ezeebits.com"
	limiter := NewInMemoryZoneRateLimiter(RateLimitPolicy{Default: RateLimit{Hits: 2, Window: time.Minute}})

	current := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return current }

	// 2 hits allowed within the window.
	for i := 1; i <= 2; i++ {
		allowed, _, err := limiter.Acquire(context.Background(), zone)
		if err != nil || !allowed {
			t.Fatalf("hit %d: expected allowed, got allowed=%v err=%v", i, allowed, err)
		}
	}

	// 3rd hit is over quota -> blocked, with a positive retryAfter.
	allowed, retryAfter, err := limiter.Acquire(context.Background(), zone)
	if err != nil {
		t.Fatalf("hit 3: unexpected error %v", err)
	}
	if allowed {
		t.Fatalf("hit 3: expected blocked, got allowed")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("hit 3: expected retryAfter within (0, 1m], got %s", retryAfter)
	}

	// Advance past the window -> quota resets, next hit allowed again.
	current = current.Add(time.Minute)
	allowed, _, err = limiter.Acquire(context.Background(), zone)
	if err != nil || !allowed {
		t.Fatalf("after window reset: expected allowed, got allowed=%v err=%v", allowed, err)
	}
}

// TestInMemoryLimiterZonesAreIndependent verifies one zone's exhausted quota
// does not block a different zone.
func TestInMemoryLimiterZonesAreIndependent(t *testing.T) {
	limiter := NewInMemoryZoneRateLimiter(RateLimitPolicy{Default: RateLimit{Hits: 1, Window: time.Minute}})
	current := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return current }

	zoneA, zoneB := "http://app.ezeebits.com", "http://app.r2.ezeebits.com"

	if allowed, _, _ := limiter.Acquire(context.Background(), zoneA); !allowed {
		t.Fatal("zoneA first hit should be allowed")
	}
	if allowed, _, _ := limiter.Acquire(context.Background(), zoneA); allowed {
		t.Fatal("zoneA second hit should be blocked")
	}
	// zoneB must still be allowed even though zoneA is exhausted.
	if allowed, _, _ := limiter.Acquire(context.Background(), zoneB); !allowed {
		t.Fatal("zoneB should be allowed while zoneA is rate-limited")
	}
}

// TestParseRateLimit checks the hits/window config format.
func TestParseRateLimit(t *testing.T) {
	valid := map[string]RateLimit{
		"10/1m": {Hits: 10, Window: time.Minute},
		"5/30s": {Hits: 5, Window: 30 * time.Second},
	}
	for input, want := range valid {
		got, err := parseRateLimit(input)
		if err != nil || got != want {
			t.Fatalf("parseRateLimit(%q) = %+v, %v; want %+v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "10", "0/1m", "-1/1m", "10/0s", "abc/1m", "10/xyz"} {
		if _, err := parseRateLimit(input); err == nil {
			t.Fatalf("parseRateLimit(%q): expected error, got nil", input)
		}
	}
}
