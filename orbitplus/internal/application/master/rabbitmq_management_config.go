package master

import (
	"fmt"
	"net/url"
	"os"
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

func loadRabbitMQManagementConfig() (*RabbitMQManagementConfig, error) {
	baseURL := strings.TrimSpace(os.Getenv("RABBITMQ_MANAGEMENT_URL"))
	username := os.Getenv("RABBITMQ_MANAGEMENT_USERNAME")
	password := os.Getenv("RABBITMQ_MANAGEMENT_PASSWORD")
	vhost := os.Getenv("RABBITMQ_MANAGEMENT_VHOST")
	timeoutValue := os.Getenv("RABBITMQ_MANAGEMENT_TIMEOUT")
	if baseURL == "" && username == "" && password == "" && vhost == "" && timeoutValue == "" {
		return nil, nil
	}
	if baseURL == "" || username == "" || password == "" {
		return nil, fmt.Errorf("RABBITMQ_MANAGEMENT_URL, RABBITMQ_MANAGEMENT_USERNAME, and RABBITMQ_MANAGEMENT_PASSWORD must all be set")
	}
	parsedURL, err := url.ParseRequestURI(baseURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("RABBITMQ_MANAGEMENT_URL must be an absolute HTTP or HTTPS URL")
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
	return &RabbitMQManagementConfig{URL: strings.TrimRight(baseURL, "/"), Username: username, Password: password, VHost: vhost, Timeout: timeout}, nil
}
