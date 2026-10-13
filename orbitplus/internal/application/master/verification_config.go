package master

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// maxBitsHTTPTimeout bounds how long one live Bits fetch may occupy a
// verification slot. It caps worst-case outbound load together with the
// concurrency limit.
const maxBitsHTTPTimeout = 30 * time.Second

// Defaults for the optional tuning variables, so BITS_BASE_URL on its own is
// enough to enable the feature.
const (
	defaultBitsHTTPTimeout           = 10 * time.Second
	defaultVerificationMaxConcurrent = 4
)

var defaultMasterBitsRateLimit = RateLimit{Hits: 10, Window: time.Minute}

// VerificationConfig holds the settings enabling live Bits verification.
//
// It carries neither credentials nor an endpoint. Credentials and the zone
// come from the read request, so nothing here is a secret and nothing here can
// point the service at the wrong zone.
type VerificationConfig struct {
	HTTPTimeout   time.Duration
	MaxConcurrent int
	RateLimit     RateLimitPolicy
}

// loadVerificationConfig reads the live verification tuning group.
//
// There is nothing to enable here: the live path is available whenever storage
// exists, because comparison and repair depend on the persisted copy. The
// endpoint is chosen per request from the zone, and credentials are bound from
// the read route.
//
// BITS_HTTP_TIMEOUT and LIVE_VERIFICATION_MAX_CONCURRENT are optional tuning.
// Unset means the default; set but unparseable is an error rather than a silent
// fallback, since a typo there would quietly change outbound load.
func loadVerificationConfig(_ AppEnvironment, storage *StorageConfig) (*VerificationConfig, error) {
	timeout := defaultBitsHTTPTimeout
	if rawTimeout := strings.TrimSpace(os.Getenv("BITS_HTTP_TIMEOUT")); rawTimeout != "" {
		parsed, err := time.ParseDuration(rawTimeout)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("BITS_HTTP_TIMEOUT must be a positive duration")
		}
		if parsed > maxBitsHTTPTimeout {
			return nil, fmt.Errorf("BITS_HTTP_TIMEOUT must not exceed %s", maxBitsHTTPTimeout)
		}
		timeout = parsed
	}

	maxConcurrent := defaultVerificationMaxConcurrent
	if rawConcurrency := strings.TrimSpace(os.Getenv("LIVE_VERIFICATION_MAX_CONCURRENT")); rawConcurrency != "" {
		parsed, err := strconv.Atoi(rawConcurrency)
		if err != nil || parsed < 1 {
			return nil, fmt.Errorf("LIVE_VERIFICATION_MAX_CONCURRENT must be an integer of at least 1")
		}
		maxConcurrent = parsed
	}

	rateLimit := defaultMasterBitsRateLimit
	if rawRateLimit := strings.TrimSpace(os.Getenv("MASTER_BITS_RATE_LIMIT")); rawRateLimit != "" {
		parsed, err := parseMasterRateLimit(rawRateLimit)
		if err != nil {
			return nil, fmt.Errorf("MASTER_BITS_RATE_LIMIT %w", err)
		}
		rateLimit = parsed
	}
	rateLimitOverrides, err := parseMasterRateLimitOverrides(strings.TrimSpace(os.Getenv("MASTER_BITS_RATE_LIMIT_OVERRIDES")))
	if err != nil {
		return nil, err
	}

	if storage == nil {
		return nil, fmt.Errorf("live verification requires Cassandra/storage configuration (CASSANDRA_HOSTS)")
	}

	return &VerificationConfig{
		HTTPTimeout:   timeout,
		MaxConcurrent: maxConcurrent,
		RateLimit: RateLimitPolicy{
			Default:   rateLimit,
			Overrides: rateLimitOverrides,
		},
	}, nil
}

func parseMasterRateLimit(value string) (RateLimit, error) {
	hitsText, windowText, found := strings.Cut(strings.TrimSpace(value), "/")
	if !found {
		return RateLimit{}, fmt.Errorf("must be in hits/window form, e.g. 10/1m")
	}
	hits, err := strconv.Atoi(strings.TrimSpace(hitsText))
	if err != nil || hits <= 0 {
		return RateLimit{}, fmt.Errorf("must have a positive hit count, e.g. 10/1m")
	}
	window, err := time.ParseDuration(strings.TrimSpace(windowText))
	if err != nil || window < time.Millisecond {
		return RateLimit{}, fmt.Errorf("must have a window duration of at least 1ms, e.g. 10/1m")
	}
	return RateLimit{Hits: hits, Window: window}, nil
}

func parseMasterRateLimitOverrides(value string) (map[string]RateLimit, error) {
	overrides := make(map[string]RateLimit)
	if value == "" {
		return overrides, nil
	}
	for _, pair := range strings.Split(value, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		separator := strings.LastIndex(pair, "=")
		if separator <= 0 || separator == len(pair)-1 {
			return nil, fmt.Errorf("MASTER_BITS_RATE_LIMIT_OVERRIDES must be comma-separated zoneURL=hits/window pairs")
		}
		zone := strings.TrimSpace(pair[:separator])
		limit, err := parseMasterRateLimit(pair[separator+1:])
		if err != nil {
			return nil, fmt.Errorf("MASTER_BITS_RATE_LIMIT_OVERRIDES %w", err)
		}
		overrides[zone] = limit
	}
	return overrides, nil
}

// ValidateBitsURL permits https in every environment and http only outside
// production. Userinfo, a query, and a fragment are rejected because the
// adapter appends path segments and would silently discard them.
//
// It is applied to the zone endpoint on every live fetch rather than once at
// startup, because the endpoint is now chosen per request.
func ValidateBitsURL(rawURL string, environment AppEnvironment) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("zone endpoint must be a valid endpoint without user information, query, or fragment")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && environment != Production {
		return nil
	}
	if environment == Production {
		return fmt.Errorf("BITS_BASE_URL must use https")
	}
	return fmt.Errorf("BITS_BASE_URL must use https or http")
}
