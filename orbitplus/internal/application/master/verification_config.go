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

// Bits credentials are compiled in, matching orbitplusworker's
// temporaryBitsUsername and temporaryBitsAPIToken, so the two modules
// authenticate to Bits as the same principal and only the endpoint has to be
// supplied at run time.
//
// TODO: Replace this temporary hardcoding once the credential API exists. Both
// modules must change together, or master and the Worker will disagree about
// who they are to Bits.
const temporaryBitsUsername = "ram"
const temporaryBitsAPIToken = "85827049535E9525097UJ16"

// Defaults for the optional tuning variables, so BITS_BASE_URL on its own is
// enough to enable the feature.
const (
	defaultBitsHTTPTimeout           = 10 * time.Second
	defaultVerificationMaxConcurrent = 4
)

// VerificationConfig holds the settings enabling live Bits verification.
//
// Credentials live here and in the Bits adapter only. They are never written to
// a log line, an error, a response body, or a persisted row. They are filled
// from the compiled-in constants rather than the environment, so the adapter
// keeps taking them as fields and stays unaware of where they came from.
type VerificationConfig struct {
	BitsBaseURL   string
	BitsUsername  string
	BitsAPIToken  string
	HTTPTimeout   time.Duration
	MaxConcurrent int
}

// loadVerificationConfig reads the live verification environment group.
//
// BITS_BASE_URL alone is the switch: unset means the feature is disabled, which
// is the default because the read routes it hangs off are unauthenticated.
// Credentials are not read from the environment at all, they come from the
// compiled-in constants, so an operator only has to supply the endpoint.
//
// BITS_HTTP_TIMEOUT and LIVE_VERIFICATION_MAX_CONCURRENT are optional tuning.
// Unset means the default; set but unparseable is an error rather than a silent
// fallback, since a typo there would quietly change outbound load.
func loadVerificationConfig(environment AppEnvironment, storage *StorageConfig) (*VerificationConfig, error) {
	baseURL := strings.TrimSpace(os.Getenv("BITS_BASE_URL"))
	if baseURL == "" {
		return nil, nil
	}
	if err := ValidateBitsURL(baseURL, environment); err != nil {
		return nil, err
	}

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
		BitsBaseURL:   baseURL,
		BitsUsername:  temporaryBitsUsername,
		BitsAPIToken:  temporaryBitsAPIToken,
		HTTPTimeout:   timeout,
		MaxConcurrent: maxConcurrent,
	}, nil
}

// ValidateBitsURL permits https in every environment and http only outside
// production. Userinfo, a query, and a fragment are rejected because the
// adapter appends path segments and would silently discard them.
func ValidateBitsURL(rawURL string, environment AppEnvironment) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("BITS_BASE_URL must be a valid endpoint without user information, query, or fragment")
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
