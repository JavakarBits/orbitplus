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

// VerificationConfig holds the settings enabling live Bits verification.
//
// It carries neither credentials nor an endpoint. Credentials and the zone
// come from the read request, so nothing here is a secret and nothing here can
// point the service at the wrong zone.
type VerificationConfig struct {
	HTTPTimeout   time.Duration
	MaxConcurrent int
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

	if storage == nil {
		return nil, fmt.Errorf("live verification requires Cassandra/storage configuration (CASSANDRA_HOSTS)")
	}

	return &VerificationConfig{
		HTTPTimeout:   timeout,
		MaxConcurrent: maxConcurrent,
	}, nil
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
