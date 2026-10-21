package master

import (
	"errors"
	"sync/atomic"
	"time"
)

// cacheSettingsConfigKey is the fixed primary key of the single cache_settings
// config row.
const cacheSettingsConfigKey = "default"

// istTimeZone is the fixed timezone for the cache window. The project operates
// on Indian Standard Time, so the window hours are always interpreted in IST
// regardless of the server's clock.
const istTimeZone = "Asia/Kolkata"

var (
	// ErrInvalidCacheSettings means submitted settings are out of range.
	ErrInvalidCacheSettings = errors.New("invalid cache settings")
	// ErrCacheSettingsNotConfigured means no store is available.
	ErrCacheSettingsNotConfigured = errors.New("cache settings are not configured")
)

// CacheSettings is the runtime-editable cache behaviour. It carries the Hot/Warm
// age thresholds and an optional daily cache window (in IST minutes-from-
// midnight). Cold is implied as "older than Warm".
type CacheSettings struct {
	HotMaxAge  time.Duration `json:"-"`
	WarmMaxAge time.Duration `json:"-"`
	// WindowEnabled turns the daily cache window on. When off, caching behaves
	// normally all day.
	WindowEnabled bool `json:"windowEnabled"`
	// WindowStartMinute / WindowEndMinute are minutes from midnight IST (0..1439)
	// marking the "cache hours" window. Inside => serve cache; outside => live.
	// If start <= end the window is a normal same-day span; if start > end it
	// wraps past midnight (e.g. 22:00..06:00).
	WindowStartMinute int `json:"windowStartMinute"`
	WindowEndMinute   int `json:"windowEndMinute"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// Validate checks the settings are sane: positive thresholds with Hot <= Warm,
// and window minutes within a day.
func (settings CacheSettings) Validate() error {
	if settings.HotMaxAge <= 0 || settings.WarmMaxAge <= 0 {
		return ErrInvalidCacheSettings
	}
	if settings.HotMaxAge > settings.WarmMaxAge {
		return ErrInvalidCacheSettings
	}
	if settings.WindowStartMinute < 0 || settings.WindowStartMinute > 1439 ||
		settings.WindowEndMinute < 0 || settings.WindowEndMinute > 1439 {
		return ErrInvalidCacheSettings
	}
	return nil
}

// CacheHoursActive reports whether the given instant falls inside the daily
// cache window, evaluated in IST. When the window is disabled it is always
// active (cache behaves normally all day). A failure to load IST falls back to
// active so caching is never accidentally disabled by a timezone error.
func (settings CacheSettings) CacheHoursActive(now time.Time) bool {
	if !settings.WindowEnabled {
		return true
	}
	location, err := time.LoadLocation(istTimeZone)
	if err != nil {
		return true
	}
	istNow := now.In(location)
	minute := istNow.Hour()*60 + istNow.Minute()
	start, end := settings.WindowStartMinute, settings.WindowEndMinute
	if start == end {
		// Zero-length window: treat as always inside (no effect).
		return true
	}
	if start < end {
		return minute >= start && minute < end
	}
	// Wraps past midnight, e.g. 22:00..06:00.
	return minute >= start || minute < end
}

// liveCacheSettings holds the currently-active settings as an atomic value so
// the read path reads them without locking. classifyFreshness and the read
// service consult this; it is seeded from env defaults at startup and replaced
// when the admin saves new settings.
var liveCacheSettings atomic.Value // stores CacheSettings

func init() {
	liveCacheSettings.Store(CacheSettings{
		HotMaxAge:  hotMaxAge,
		WarmMaxAge: warmMaxAge,
	})
}

// activeCacheSettings returns the currently-active settings snapshot.
func activeCacheSettings() CacheSettings {
	if value, ok := liveCacheSettings.Load().(CacheSettings); ok {
		return value
	}
	return CacheSettings{HotMaxAge: hotMaxAge, WarmMaxAge: warmMaxAge}
}

// setActiveCacheSettings replaces the live settings snapshot. Called at startup
// after loading from the store, and whenever the admin saves.
func setActiveCacheSettings(settings CacheSettings) {
	liveCacheSettings.Store(settings)
}
