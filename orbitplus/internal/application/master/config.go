package master

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// AppEnvironment identifies the deployment environment the process runs in.
type AppEnvironment string

const (
	Development AppEnvironment = "development"
	Production  AppEnvironment = "production"
)

// RuntimeConfig is the service runtime configuration.
type RuntimeConfig struct {
	AppEnvironment          AppEnvironment
	APIPort                 int
	UIAccessToken           string
	Storage                 *StorageConfig
	Queue                   *QueueConfig
	PeriodicRefreshInterval time.Duration
}

// DefaultRuntimeConfig returns the configuration used when no environment
// variables are set.
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		AppEnvironment: Development,
		APIPort:        8082,
	}
}

// LoadRuntimeConfig reads APP_ENV, MASTER_API_PORT, and optional datastore
// configuration for persisted TripDetails reads and writes.
func LoadRuntimeConfig() (RuntimeConfig, error) {
	config := DefaultRuntimeConfig()

	if value := os.Getenv("APP_ENV"); value != "" {
		config.AppEnvironment = AppEnvironment(value)
	}

	if value := os.Getenv("MASTER_API_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return RuntimeConfig{}, fmt.Errorf("MASTER_API_PORT must be an integer between 1 and 65535")
		}
		config.APIPort = port
	}
	config.UIAccessToken = os.Getenv("ORBITPLUS_UI_ACCESS_TOKEN")

	persistence, err := loadStorageConfig()
	if err != nil {
		return RuntimeConfig{}, err
	}
	config.Storage = persistence
	queue, err := loadQueueConfig()
	if err != nil {
		return RuntimeConfig{}, err
	}
	config.Queue = queue
	if value := os.Getenv("PERIODIC_REFRESH_INTERVAL"); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil || interval <= 0 {
			return RuntimeConfig{}, fmt.Errorf("PERIODIC_REFRESH_INTERVAL must be a positive duration")
		}
		if config.Storage == nil {
			return RuntimeConfig{}, fmt.Errorf("PERIODIC_REFRESH_INTERVAL requires Cassandra/storage configuration")
		}
		if config.Queue == nil {
			return RuntimeConfig{}, fmt.Errorf("PERIODIC_REFRESH_INTERVAL requires RabbitMQ configuration")
		}
		config.PeriodicRefreshInterval = interval
	}
	return config, nil
}

// Address returns the host:port the HTTP server should listen on.
func (config RuntimeConfig) Address() string {
	return fmt.Sprintf(":%d", config.APIPort)
}
