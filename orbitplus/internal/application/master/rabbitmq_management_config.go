package master

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// RabbitMQManagementConfig contains read-only RabbitMQ Management API settings.
type RabbitMQManagementConfig struct {
	URL      string
	Username string
	Password string
	VHost    string
	Timeout  time.Duration
}

// loadRabbitMQManagementConfig builds the read-only Management API config from
// discrete parts only. The endpoint is assembled from RABBITMQ_MANAGEMENT_HOST
// (falling back to RABBITMQ_HOST), RABBITMQ_MANAGEMENT_PORT (default 15672),
// and RABBITMQ_MANAGEMENT_TLS (https when "true"). There is no
// RABBITMQ_MANAGEMENT_URL and no URL to parse or validate. The feature stays
// disabled when no management setting is present, because it is only a
// monitoring dashboard and is never required for publishing.
func loadRabbitMQManagementConfig() (*RabbitMQManagementConfig, error) {
	username := os.Getenv("RABBITMQ_MANAGEMENT_USERNAME")
	password := os.Getenv("RABBITMQ_MANAGEMENT_PASSWORD")
	vhost := os.Getenv("RABBITMQ_MANAGEMENT_VHOST")
	timeoutValue := os.Getenv("RABBITMQ_MANAGEMENT_TIMEOUT")
	mgmtHost := strings.TrimSpace(os.Getenv("RABBITMQ_MANAGEMENT_HOST"))
	mgmtPort := os.Getenv("RABBITMQ_MANAGEMENT_PORT")
	mgmtTLS := os.Getenv("RABBITMQ_MANAGEMENT_TLS")

	if username == "" && password == "" && vhost == "" && timeoutValue == "" &&
		mgmtHost == "" && mgmtPort == "" && mgmtTLS == "" {
		return nil, nil
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("RABBITMQ_MANAGEMENT_USERNAME and RABBITMQ_MANAGEMENT_PASSWORD must be set to enable the management dashboard")
	}
	baseURL, err := buildManagementURL(mgmtHost)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		return nil, fmt.Errorf("set RABBITMQ_MANAGEMENT_HOST or RABBITMQ_HOST to enable the management dashboard")
	}
	if vhost == "" {
		vhost = "/"
	}
	timeout := 5 * time.Second
	if timeoutValue != "" {
		timeout, err = time.ParseDuration(timeoutValue)
		if err != nil || timeout <= 0 {
			return nil, fmt.Errorf("RABBITMQ_MANAGEMENT_TIMEOUT must be a positive duration")
		}
	}
	return &RabbitMQManagementConfig{URL: baseURL, Username: username, Password: password, VHost: vhost, Timeout: timeout}, nil
}

// buildManagementURL assembles the Management API base URL from parts. The host
// defaults to RABBITMQ_HOST, the port to 15672, and the scheme to http (https
// when RABBITMQ_MANAGEMENT_TLS is "true"). Returns "" when no host is available.
func buildManagementURL(host string) (string, error) {
	if host == "" {
		host = strings.TrimSpace(os.Getenv("RABBITMQ_HOST"))
	}
	if host == "" {
		return "", nil
	}
	port := 15672
	if value := os.Getenv("RABBITMQ_MANAGEMENT_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return "", fmt.Errorf("RABBITMQ_MANAGEMENT_PORT must be an integer between 1 and 65535")
		}
		port = parsed
	}
	scheme := "http"
	if strings.EqualFold(strings.TrimSpace(os.Getenv("RABBITMQ_MANAGEMENT_TLS")), "true") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port), nil
}
