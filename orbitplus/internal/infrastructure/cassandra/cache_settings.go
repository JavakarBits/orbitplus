package cassandra

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/application/master"
)

const cacheSettingsTable = "cache_settings"
const cacheSettingsConfigKey = "default"

// CacheSettingsRepository persists the single runtime cache-settings row.
type CacheSettingsRepository struct {
	session *gocql.Session
}

// NewCacheSettingsRepository opens a session for the cache_settings table.
func NewCacheSettingsRepository(ctx context.Context, config Config) (*CacheSettingsRepository, error) {
	if !validIdentifier(config.Keyspace) {
		return nil, fmt.Errorf("invalid Cassandra keyspace")
	}
	cluster := gocql.NewCluster(config.Hosts...)
	cluster.Port = config.Port
	cluster.Keyspace = config.Keyspace
	cluster.Timeout = config.Timeout
	cluster.ConnectTimeout = config.Timeout
	if config.Username != "" || config.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{Username: config.Username, Password: config.Password}
	}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connect to Cassandra: %w", err)
	}
	return &CacheSettingsRepository{session: session}, nil
}

// LoadCacheSettings reads the single config row. Returns found=false when no row
// exists yet (first run), so the caller keeps the env defaults.
func (repository *CacheSettingsRepository) LoadCacheSettings(ctx context.Context) (master.CacheSettings, bool, error) {
	const query = `SELECT hot_max_age_seconds, warm_max_age_seconds, window_enabled,
		window_start_minute, window_end_minute, updated_at
		FROM ` + cacheSettingsTable + ` WHERE config_key=?`
	var hotSeconds, warmSeconds int64
	var windowEnabled bool
	var windowStart, windowEnd int
	var updatedAt time.Time
	err := repository.session.Query(query, cacheSettingsConfigKey).WithContext(ctx).
		Scan(&hotSeconds, &warmSeconds, &windowEnabled, &windowStart, &windowEnd, &updatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return master.CacheSettings{}, false, nil
	}
	if err != nil {
		return master.CacheSettings{}, false, fmt.Errorf("load cache settings: %w", err)
	}
	return master.CacheSettings{
		HotMaxAge:         time.Duration(hotSeconds) * time.Second,
		WarmMaxAge:        time.Duration(warmSeconds) * time.Second,
		WindowEnabled:     windowEnabled,
		WindowStartMinute: windowStart,
		WindowEndMinute:   windowEnd,
		UpdatedAt:         updatedAt,
	}, true, nil
}

// SaveCacheSettings upserts the single config row.
func (repository *CacheSettingsRepository) SaveCacheSettings(ctx context.Context, settings master.CacheSettings) error {
	const query = `INSERT INTO ` + cacheSettingsTable + `
		(config_key, hot_max_age_seconds, warm_max_age_seconds, window_enabled,
		window_start_minute, window_end_minute, window_timezone, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, cacheSettingsConfigKey,
		int64(settings.HotMaxAge/time.Second), int64(settings.WarmMaxAge/time.Second),
		settings.WindowEnabled, settings.WindowStartMinute, settings.WindowEndMinute,
		"Asia/Kolkata", settings.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save cache settings: %w", err)
	}
	return nil
}

// Close releases the Cassandra connection.
func (repository *CacheSettingsRepository) Close() {
	if repository != nil && repository.session != nil {
		repository.session.Close()
	}
}
