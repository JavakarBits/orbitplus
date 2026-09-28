package master

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// QueueConfig contains RabbitMQ publishing configuration.
type QueueConfig struct {
	URL      string
	Exchange string
}

// loadQueueConfig builds the RabbitMQ publishing config. The connection can be
// supplied either as a single RABBITMQ_URL, or as discrete parts
// (RABBITMQ_HOST with optional RABBITMQ_PORT/RABBITMQ_USER/RABBITMQ_PASSWORD/
// RABBITMQ_VHOST). RABBITMQ_URL wins when both are present. RABBITMQ_EXCHANGE
// is always required. Publishing stays disabled only when nothing is set.
func loadQueueConfig() (*QueueConfig, error) {
	rawURL := os.Getenv("RABBITMQ_URL")
	host := os.Getenv("RABBITMQ_HOST")
	exchange := os.Getenv("RABBITMQ_EXCHANGE")
	if rawURL == "" && host == "" && exchange == "" {
		return nil, nil
	}
	if rawURL == "" {
		built, err := buildAMQPURL(host)
		if err != nil {
			return nil, err
		}
		rawURL = built
	}
	if exchange == "" {
		return nil, fmt.Errorf("RABBITMQ_EXCHANGE must be set for inventory event publishing")
	}
	return &QueueConfig{URL: rawURL, Exchange: exchange}, nil
}

// buildAMQPURL assembles an amqp URL from discrete parts when RABBITMQ_URL is
// not provided. RABBITMQ_HOST is required; the rest have amqp defaults
// (port 5672, default vhost "/"). url.UserPassword escapes credentials so a
// password with special characters is encoded correctly.
func buildAMQPURL(host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("set RABBITMQ_URL, or RABBITMQ_HOST (with optional RABBITMQ_PORT/RABBITMQ_USER/RABBITMQ_PASSWORD) for inventory event publishing")
	}
	port := 5672
	if value := os.Getenv("RABBITMQ_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return "", fmt.Errorf("RABBITMQ_PORT must be an integer between 1 and 65535")
		}
		port = parsed
	}
	amqpURL := url.URL{
		Scheme: "amqp",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/",
	}
	user := os.Getenv("RABBITMQ_USER")
	password := os.Getenv("RABBITMQ_PASSWORD")
	if user != "" || password != "" {
		amqpURL.User = url.UserPassword(user, password)
	}
	if vhost := os.Getenv("RABBITMQ_VHOST"); vhost != "" {
		amqpURL.Path = "/" + vhost
	}
	return amqpURL.String(), nil
}
