package master

import (
	"os"
	"strings"
	"time"
)

// OrbitCredentialConfig locates the Orbit credential service the Master uses to
// resolve an operator's BITS credential at registration time: base URL +
// namespace + shared access token. When any required value is empty the
// feature is disabled (nil).
type OrbitCredentialConfig struct {
	BaseURL       string
	NamespaceCode string
	AccessToken   string
	Timeout       time.Duration
}

const defaultOrbitCredentialTimeout = 5 * time.Second

// orbitEnv reads a shared ORBIT_<suffix> variable, falling back to a legacy
// name for backward compatibility. The Orbit route and credential calls use the
// same host/token/namespace, so they share one ORBIT_* block.
func orbitEnv(sharedName, legacyName string) string {
	if value := strings.TrimSpace(os.Getenv(sharedName)); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv(legacyName))
}

func loadOrbitCredentialConfig() *OrbitCredentialConfig {
	baseURL := orbitEnv("ORBIT_BASE_URL", "ORBIT_CREDENTIAL_BASE_URL")
	namespaceCode := orbitEnv("ORBIT_NAMESPACE_CODE", "ORBIT_CREDENTIAL_NAMESPACE_CODE")
	accessToken := orbitEnv("ORBIT_ACCESS_TOKEN", "ORBIT_CREDENTIAL_ACCESS_TOKEN")
	if baseURL == "" || namespaceCode == "" || accessToken == "" {
		return nil
	}
	if !validHTTPURL(baseURL) {
		return nil
	}
	timeout := defaultOrbitCredentialTimeout
	if raw := orbitEnv("ORBIT_TIMEOUT", "ORBIT_CREDENTIAL_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil
		}
		timeout = parsed
	}
	return &OrbitCredentialConfig{
		BaseURL:       baseURL,
		NamespaceCode: namespaceCode,
		AccessToken:   accessToken,
		Timeout:       timeout,
	}
}
