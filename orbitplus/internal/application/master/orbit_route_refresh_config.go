package master

import (
	"net/url"
	"os"
	"strings"
	"time"
)

// OrbitRouteRefreshConfig contains runtime settings for stale route refresh.
type OrbitRouteRefreshConfig struct {
	BaseURL       string
	AccessToken   string
	Timeout       time.Duration
	Interval      time.Duration
	StaleDuration time.Duration
}

func loadOrbitRouteRefreshConfig() (*OrbitRouteRefreshConfig, error) {
	baseURL := strings.TrimSpace(os.Getenv("ORBIT_ROUTE_BASE_URL"))
	accessToken := strings.TrimSpace(os.Getenv("ORBIT_ROUTE_ACCESS_TOKEN"))
	timeout := strings.TrimSpace(os.Getenv("ORBIT_ROUTE_TIMEOUT"))
	interval := strings.TrimSpace(os.Getenv("ORBIT_ROUTE_REFRESH_INTERVAL"))
	staleDuration := strings.TrimSpace(os.Getenv("ORBIT_ROUTE_STALE_DURATION"))
	if baseURL == "" || accessToken == "" || timeout == "" || interval == "" || staleDuration == "" {
		return nil, nil
	}
	if !validHTTPURL(baseURL) {
		return nil, nil
	}
	requestTimeout, err := time.ParseDuration(timeout)
	if err != nil || requestTimeout <= 0 {
		return nil, nil
	}
	refreshInterval, err := time.ParseDuration(interval)
	if err != nil || refreshInterval <= 0 {
		return nil, nil
	}
	refreshStaleDuration, err := time.ParseDuration(staleDuration)
	if err != nil || refreshStaleDuration <= 0 {
		return nil, nil
	}
	return &OrbitRouteRefreshConfig{
		BaseURL: baseURL, AccessToken: accessToken, Timeout: requestTimeout,
		Interval: refreshInterval, StaleDuration: refreshStaleDuration,
	}, nil
}

func validHTTPURL(value string) bool {
	parsedURL, err := url.ParseRequestURI(value)
	return err == nil && parsedURL.Host != "" && (parsedURL.Scheme == "http" || parsedURL.Scheme == "https")
}
