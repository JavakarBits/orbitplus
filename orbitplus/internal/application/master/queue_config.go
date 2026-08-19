package master

import (
	"fmt"
	"os"
)

// QueueConfig contains RabbitMQ publishing configuration.
type QueueConfig struct {
	URL      string
	Exchange string
}

func loadQueueConfig() (*QueueConfig, error) {
	url := os.Getenv("RABBITMQ_URL")
	exchange := os.Getenv("RABBITMQ_EXCHANGE")
	if url == "" && exchange == "" {
		return nil, nil
	}
	if url == "" || exchange == "" {
		return nil, fmt.Errorf("RABBITMQ_URL and RABBITMQ_EXCHANGE must both be set for inventory event publishing")
	}
	return &QueueConfig{URL: url, Exchange: exchange}, nil
}
