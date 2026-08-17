package master

import (
	"fmt"
	"os"
	"strconv"
)

// AppEnvironment identifies the deployment environment the process runs in.
type AppEnvironment string

const (
	Development AppEnvironment = "development"
	Production  AppEnvironment = "production"
)

// RuntimeConfig is the minimal Phase 1 configuration for orbitplusmaster.
type RuntimeConfig struct {
	AppEnvironment AppEnvironment
	APIPort        int
}

// DefaultRuntimeConfig returns the configuration used when no environment
// variables are set.
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		AppEnvironment: Development,
		APIPort:        8082,
	}
}

// LoadRuntimeConfig reads Phase 1 configuration from environment variables.
// Only APP_ENV and MASTER_API_PORT are supported in this phase.
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

	return config, nil
}

// Address returns the host:port the HTTP server should listen on.
func (config RuntimeConfig) Address() string {
	return fmt.Sprintf(":%d", config.APIPort)
}
