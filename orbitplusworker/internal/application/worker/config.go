package worker

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidConfig = errors.New("invalid worker configuration")

// AppEnvironment selects the deployment transport policy.
type AppEnvironment string

const (
	Development AppEnvironment = "development"
	Production  AppEnvironment = "production"
)

func (environment AppEnvironment) Validate() error {
	switch environment {
	case Development, Production:
		return nil
	default:
		return fmt.Errorf("%w: APP_ENV must be development or production", ErrInvalidConfig)
	}
}

func (environment AppEnvironment) AllowsInsecureEndpoints() bool {
	return environment == Development
}

// WorkerConfig defines the maximum process-local concurrent operations.
type WorkerConfig struct {
	WorkerConcurrency int `json:"workerConcurrency"`
}

func (config WorkerConfig) Validate() error {
	if config.WorkerConcurrency <= 0 {
		return fmt.Errorf("%w: worker concurrency must be positive", ErrInvalidConfig)
	}
	return nil
}

// HealthAPIConfig defines the listener used for dependency-independent health checks.
type HealthAPIConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (config HealthAPIConfig) Address() string {
	return net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
}

func (config HealthAPIConfig) Validate() error {
	host := strings.TrimSpace(config.Host)
	if host == "" || host != config.Host || !validHealthAPIHost(host) {
		return fmt.Errorf("health API host must be a valid IP address or hostname")
	}
	if config.Port < 1 || config.Port > 65535 {
		return fmt.Errorf("health API port must be between 1 and 65535")
	}
	return nil
}

func validHealthAPIHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character != '-' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
				return false
			}
		}
	}
	return true
}

// TLSFileConfig names deployment-managed certificate files; it contains no
// certificate or key material itself.
type TLSFileConfig struct {
	CAFile         string `json:"caFile"`
	ClientCertFile string `json:"clientCertFile"`
	ClientKeyFile  string `json:"clientKeyFile"`
	ServerName     string `json:"serverName"`
}

// HTTPAuthConfig contains runtime HTTP authentication material resolved from an
// environment variable or a mounted secret file, never from JSON configuration.
type HTTPAuthConfig struct {
	BearerToken string `json:"-"`
}

type RabbitMQConfig struct {
	URL        string        `json:"url"`
	Queue      string        `json:"queue"`
	Exchange   string        `json:"exchange"`
	RoutingKey string        `json:"routingKey"`
	Prefetch   int           `json:"prefetch"`
	TLS        TLSFileConfig `json:"tls"`
	Username   string        `json:"-"`
	Password   string        `json:"-"`
}

type BitsConfig struct {
	BaseURL string        `json:"baseUrl"`
	TLS     TLSFileConfig `json:"tls"`
}

type OrbitPlusConfig struct {
	Endpoint string         `json:"endpoint"`
	TLS      TLSFileConfig  `json:"tls"`
	Auth     HTTPAuthConfig `json:"-"`
}

// RuntimeConfig is the validated composition input. Secret values are resolved
// only at startup from environment variables or mounted files.
type RuntimeConfig struct {
	AppEnvironment        AppEnvironment  `json:"-"`
	RabbitMQ              RabbitMQConfig  `json:"rabbitmq"`
	Bits                  BitsConfig      `json:"bits"`
	OrbitPlus             OrbitPlusConfig `json:"orbitPlus"`
	Worker                WorkerConfig    `json:"worker"`
	HealthAPI             HealthAPIConfig `json:"healthApi"`
	HTTPTimeout           time.Duration   `json:"httpTimeout"`
	OrbitPlusResponseSize int64           `json:"orbitPlusResponseSize"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		AppEnvironment: Production,
		RabbitMQ:       RabbitMQConfig{Prefetch: 10},
		Worker:         WorkerConfig{WorkerConcurrency: 10},
		HealthAPI:      HealthAPIConfig{Host: "0.0.0.0", Port: 8080},
		HTTPTimeout:    15 * time.Second, OrbitPlusResponseSize: 64 << 10,
	}
}

// LoadRuntimeConfig reads optional non-secret JSON settings then environment
// overrides. Values ending in _FILE are read from deployment-managed files.
func LoadRuntimeConfig() (RuntimeConfig, error) {
	config := DefaultRuntimeConfig()
	if configFilePath := os.Getenv("TRIPDETAILS_REFRESH_WORKER_CONFIG_FILE"); configFilePath != "" {
		configFile, err := os.Open(configFilePath)
		if err != nil {
			return RuntimeConfig{}, fmt.Errorf("open runtime configuration: %w", err)
		}
		defer configFile.Close()
		configDecoder := json.NewDecoder(configFile)
		configDecoder.DisallowUnknownFields()
		if err := configDecoder.Decode(&config); err != nil {
			return RuntimeConfig{}, fmt.Errorf("decode runtime configuration: %w", err)
		}
	}
	if err := config.applyEnvironment(os.LookupEnv); err != nil {
		return RuntimeConfig{}, err
	}
	return config, config.Validate()
}

func (config *RuntimeConfig) applyEnvironment(lookup func(string) (string, bool)) error {
	if value, ok := lookup("APP_ENV"); ok {
		config.AppEnvironment = AppEnvironment(value)
	}
	setString := func(name string, target *string) {
		if value, ok := lookup(name); ok && value != "" {
			*target = value
		}
	}
	setString("RABBITMQ_URL", &config.RabbitMQ.URL)
	setString("RABBITMQ_QUEUE", &config.RabbitMQ.Queue)
	setString("RABBITMQ_EXCHANGE", &config.RabbitMQ.Exchange)
	setString("RABBITMQ_ROUTING_KEY", &config.RabbitMQ.RoutingKey)
	setString("BITS_BASE_URL", &config.Bits.BaseURL)
	setString("ORBITPLUS_URL", &config.OrbitPlus.Endpoint)
	setString("HEALTH_API_HOST", &config.HealthAPI.Host)
	if err := setTCPPort(lookup, "HEALTH_API_PORT", &config.HealthAPI.Port); err != nil {
		return err
	}
	if err := loadSecret(lookup, "RABBITMQ_USERNAME", "RABBITMQ_USERNAME_FILE", &config.RabbitMQ.Username); err != nil {
		return err
	}
	if err := loadSecret(lookup, "RABBITMQ_PASSWORD", "RABBITMQ_PASSWORD_FILE", &config.RabbitMQ.Password); err != nil {
		return err
	}
	if err := loadSecret(lookup, "ORBITPLUS_BEARER_TOKEN", "ORBITPLUS_BEARER_TOKEN_FILE", &config.OrbitPlus.Auth.BearerToken); err != nil {
		return err
	}
	if err := setPositiveInt(lookup, "WORKER_CONCURRENCY", &config.Worker.WorkerConcurrency); err != nil {
		return err
	}
	if err := setPositiveInt(lookup, "RABBITMQ_PREFETCH", &config.RabbitMQ.Prefetch); err != nil {
		return err
	}
	if err := setPositiveDuration(lookup, "WORKER_HTTP_TIMEOUT", &config.HTTPTimeout); err != nil {
		return err
	}
	return nil
}

func loadSecret(lookup func(string) (string, bool), name, fileName string, target *string) error {
	value, direct := lookup(name)
	path, fromFile := lookup(fileName)
	if direct && strings.TrimSpace(value) != "" && fromFile && strings.TrimSpace(path) != "" {
		return fmt.Errorf("%s and %s are mutually exclusive", name, fileName)
	}
	if direct && strings.TrimSpace(value) != "" {
		*target = value
		return nil
	}
	if fromFile && strings.TrimSpace(path) != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", fileName, err)
		}
		if value = strings.TrimSpace(string(contents)); value == "" {
			return fmt.Errorf("%s is empty", fileName)
		}
		*target = value
	}
	return nil
}

func setPositiveInt(lookup func(string) (string, bool), name string, target *int) error {
	value, ok := lookup(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = parsed
	return nil
}

func setPositiveDuration(lookup func(string) (string, bool), name string, target *time.Duration) error {
	if value, ok := lookup(name); ok && value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("%s must be a positive duration", name)
		}
		*target = parsed
	}
	return nil
}

func setTCPPort(lookup func(string) (string, bool), name string, target *int) error {
	value, ok := lookup(name)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		return fmt.Errorf("%s must be an integer between 1 and 65535", name)
	}
	*target = parsed
	return nil
}

func (config RuntimeConfig) Validate() error {
	if err := config.AppEnvironment.Validate(); err != nil {
		return err
	}
	if err := config.WorkerConfigValid(); err != nil {
		return err
	}
	if err := config.HealthAPI.Validate(); err != nil {
		return err
	}
	if err := ValidateRabbitMQURL(config.RabbitMQ.URL, config.AppEnvironment); err != nil {
		return err
	}
	if config.RabbitMQ.Queue == "" || config.RabbitMQ.Exchange == "" || config.RabbitMQ.RoutingKey == "" || config.RabbitMQ.Prefetch <= 0 {
		return fmt.Errorf("RabbitMQ queue, exchange, routing key, and positive prefetch are required")
	}
	if (config.RabbitMQ.Username == "") != (config.RabbitMQ.Password == "") {
		return fmt.Errorf("RabbitMQ username and password must be configured together")
	}
	if err := ValidateBitsURL(config.Bits.BaseURL, config.AppEnvironment); err != nil {
		return err
	}
	if err := ValidateOrbitPlusURL(config.OrbitPlus.Endpoint, config.AppEnvironment); err != nil {
		return err
	}
	if config.HTTPTimeout <= 0 || config.OrbitPlusResponseSize <= 0 {
		return fmt.Errorf("worker limits must be valid")
	}
	return nil
}

func (config RuntimeConfig) WorkerConfigValid() error { return config.Worker.Validate() }

// ValidateRabbitMQURL permits amqps in every environment and amqp only in development.
func ValidateRabbitMQURL(raw string, environment AppEnvironment) error {
	return validateEndpointURL(raw, environment, "RabbitMQ URL", "amqps", "amqp", false, false)
}

// ValidateBitsURL permits https in every environment and http only in development.
func ValidateBitsURL(raw string, environment AppEnvironment) error {
	return validateEndpointURL(raw, environment, "Bits base URL", "https", "http", true, true)
}

// ValidateOrbitPlusURL permits https in every environment and http only in development.
func ValidateOrbitPlusURL(raw string, environment AppEnvironment) error {
	return validateEndpointURL(raw, environment, "OrbitPlus endpoint", "https", "http", false, false)
}

func validateEndpointURL(raw string, environment AppEnvironment, endpointName, secureScheme, insecureScheme string, requireHostname, rejectQueryAndFragment bool) error {
	if err := environment.Validate(); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || (requireHostname && parsed.Hostname() == "") || (rejectQueryAndFragment && (parsed.RawQuery != "" || parsed.Fragment != "")) {
		return fmt.Errorf("%s must be a valid endpoint without user information", endpointName)
	}
	if parsed.Scheme == secureScheme || (environment.AllowsInsecureEndpoints() && parsed.Scheme == insecureScheme) {
		return nil
	}
	if environment.AllowsInsecureEndpoints() {
		return fmt.Errorf("%s must use %s or %s", endpointName, secureScheme, insecureScheme)
	}
	return fmt.Errorf("%s must use %s", endpointName, secureScheme)
}

// BuildTLSConfig loads deployment-managed TLS files without exposing their contents.
func (config TLSFileConfig) BuildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: config.ServerName}
	if config.CAFile != "" {
		contents, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(contents) {
			return nil, fmt.Errorf("CA file contains no certificates")
		}
		tlsConfig.RootCAs = pool
	}
	if (config.ClientCertFile == "") != (config.ClientKeyFile == "") {
		return nil, fmt.Errorf("both client certificate and key file are required")
	}
	if config.ClientCertFile != "" {
		certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfig, nil
}
