package master

import (
	"context"
	"time"
)

// CacheSettingsStore persists the single cache-settings config row.
// *cassandra.CacheSettingsRepository implements it.
type CacheSettingsStore interface {
	LoadCacheSettings(ctx context.Context) (CacheSettings, bool, error)
	SaveCacheSettings(ctx context.Context, settings CacheSettings) error
}

// CacheSettingsService reads and writes the runtime-editable cache settings
// (Hot/Warm age thresholds + the IST daily cache window) and keeps the live
// in-memory snapshot (read by the cache read path) in sync.
type CacheSettingsService struct {
	store CacheSettingsStore
	now   func() time.Time
}

// NewCacheSettingsService constructs the service over a store. A nil store
// disables persistence, so Get returns the env-default snapshot and Save fails.
func NewCacheSettingsService(store CacheSettingsStore) *CacheSettingsService {
	return &CacheSettingsService{store: store, now: time.Now}
}

// LoadAtStartup loads any persisted settings and makes them the live snapshot.
// When no row exists yet, the env-default snapshot (seeded in init) stays
// active. A store error is returned so the caller can log it, but the live
// defaults remain usable.
func (service *CacheSettingsService) LoadAtStartup(ctx context.Context) error {
	if service == nil || service.store == nil {
		return ErrCacheSettingsNotConfigured
	}
	settings, found, err := service.store.LoadCacheSettings(ctx)
	if err != nil {
		return err
	}
	if found {
		setActiveCacheSettings(settings)
	}
	return nil
}

// Get returns the currently-active settings snapshot (what the read path uses).
func (service *CacheSettingsService) Get(ctx context.Context) (CacheSettings, error) {
	return activeCacheSettings(), nil
}

// Save validates, persists, and immediately activates the new settings.
func (service *CacheSettingsService) Save(ctx context.Context, settings CacheSettings) (CacheSettings, error) {
	if service == nil || service.store == nil {
		return CacheSettings{}, ErrCacheSettingsNotConfigured
	}
	if err := settings.Validate(); err != nil {
		return CacheSettings{}, err
	}
	settings.UpdatedAt = service.now().UTC()
	if err := service.store.SaveCacheSettings(ctx, settings); err != nil {
		return CacheSettings{}, err
	}
	setActiveCacheSettings(settings)
	return settings, nil
}
