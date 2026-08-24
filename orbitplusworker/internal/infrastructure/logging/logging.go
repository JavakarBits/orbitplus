// Package logging configures rotating file output with age-based retention.
//
// Rotated files are deleted once they are older than the configured age, so
// disk usage stays bounded without external log management. Output is written
// to the log file and to stderr by default, keeping `docker logs` and
// `kubectl logs` usable while also retaining files on disk.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Defaults applied when the matching environment variable is unset.
const (
	DefaultMaxAgeDays = 5
	DefaultMaxSizeMB  = 100
	DefaultMaxBackups = 0 // 0 means no count limit; age alone governs removal
)

// Config describes rotating log file behaviour.
type Config struct {
	// FilePath is the active log file. Rotated files are siblings of it.
	FilePath string
	// MaxAgeDays removes rotated files older than this many days.
	MaxAgeDays int
	// MaxSizeMB rotates the active file once it exceeds this size.
	MaxSizeMB int
	// MaxBackups optionally caps the number of retained rotated files.
	MaxBackups int
	// Compress gzips rotated files.
	Compress bool
	// AlsoStderr keeps container log collection working.
	AlsoStderr bool
}

// LoadConfig reads logging settings from the environment, falling back to
// defaultFilePath and the package defaults.
func LoadConfig(defaultFilePath string) (Config, error) {
	config := Config{
		FilePath:   defaultFilePath,
		MaxAgeDays: DefaultMaxAgeDays,
		MaxSizeMB:  DefaultMaxSizeMB,
		MaxBackups: DefaultMaxBackups,
		Compress:   true,
		AlsoStderr: true,
	}
	if value := strings.TrimSpace(os.Getenv("LOG_FILE_PATH")); value != "" {
		config.FilePath = value
	}
	if err := positiveInt("LOG_MAX_AGE_DAYS", &config.MaxAgeDays); err != nil {
		return Config{}, err
	}
	if err := positiveInt("LOG_MAX_SIZE_MB", &config.MaxSizeMB); err != nil {
		return Config{}, err
	}
	if err := nonNegativeInt("LOG_MAX_BACKUPS", &config.MaxBackups); err != nil {
		return Config{}, err
	}
	if err := boolean("LOG_COMPRESS", &config.Compress); err != nil {
		return Config{}, err
	}
	if err := boolean("LOG_TO_STDERR", &config.AlsoStderr); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Setup routes the standard logger, and therefore slog's default handler, to a
// rotating file. The returned function flushes and closes the file.
func Setup(config Config) (func() error, error) {
	if strings.TrimSpace(config.FilePath) == "" {
		return nil, fmt.Errorf("log file path is required")
	}
	if config.MaxAgeDays <= 0 || config.MaxSizeMB <= 0 || config.MaxBackups < 0 {
		return nil, fmt.Errorf("log retention settings must be positive")
	}
	rotator := &lumberjack.Logger{
		Filename:   config.FilePath,
		MaxAge:     config.MaxAgeDays,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxBackups,
		Compress:   config.Compress,
		LocalTime:  false, // rotated file timestamps use UTC
	}
	var writer io.Writer = rotator
	if config.AlsoStderr {
		writer = io.MultiWriter(os.Stderr, rotator)
	}
	log.SetOutput(writer)
	return rotator.Close, nil
}

func positiveInt(name string, target *int) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = parsed
	return nil
}

func nonNegativeInt(name string, target *int) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fmt.Errorf("%s must be a non-negative integer", name)
	}
	*target = parsed
	return nil
}

func boolean(name string, target *bool) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be true or false", name)
	}
	*target = parsed
	return nil
}
