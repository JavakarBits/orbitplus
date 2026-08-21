package master

import (
	"context"
	"errors"
	"strings"
)

const (
	cacheViewerDefaultPageSize int64 = 25
	cacheViewerMaximumPageSize int64 = 100
)

var (
	// ErrCacheNotConfigured indicates that Dragonfly cache access is unavailable.
	ErrCacheNotConfigured = errors.New("cache reading is not configured")
	// ErrInvalidCacheCategory indicates that the requested cache group is unsupported.
	ErrInvalidCacheCategory = errors.New("invalid cache category")
	// ErrInvalidCacheKey indicates that a cache value request has no key.
	ErrInvalidCacheKey = errors.New("invalid cache key")
)

// CacheViewer reads paginated cache keys and individual stored values.
type CacheViewer interface {
	GetJSON(context.Context, string) ([]byte, bool, error)
	ScanKeys(context.Context, uint64, string, int64) ([]string, uint64, error)
}

// CacheViewerItem identifies a cache key without returning its value.
type CacheViewerItem struct {
	Key      string
	Category string
}

// CacheViewerPage is one cursor-based page of cache keys.
type CacheViewerPage struct {
	Items      []CacheViewerItem
	NextCursor uint64
	PageSize   int64
}

// CacheValue is a separately requested cache value.
type CacheValue struct {
	Key     string
	Found   bool
	Content string
}

// CacheReadService provides read-only Dragonfly cache viewing.
type CacheReadService struct{ viewer CacheViewer }

// NewCacheReadService constructs a cache viewer service.
func NewCacheReadService(viewer CacheViewer) *CacheReadService {
	return &CacheReadService{viewer: viewer}
}

// List returns a page of cache keys for one supported group without their values.
func (service *CacheReadService) List(ctx context.Context, cursor uint64, category string, pageSize int64) (CacheViewerPage, error) {
	if service == nil || service.viewer == nil {
		return CacheViewerPage{}, ErrCacheNotConfigured
	}
	category, pattern, err := cacheCategoryPattern(category)
	if err != nil {
		return CacheViewerPage{}, err
	}
	pageSize = cacheViewerPageSize(pageSize)
	keys, nextCursor, err := service.viewer.ScanKeys(ctx, cursor, pattern, pageSize)
	if err != nil {
		return CacheViewerPage{}, err
	}
	items := make([]CacheViewerItem, 0, len(keys))
	for _, key := range keys {
		itemCategory := cacheKeyCategory(key)
		if category == "other" && itemCategory != "other" {
			continue
		}
		items = append(items, CacheViewerItem{Key: key, Category: itemCategory})
	}
	return CacheViewerPage{Items: items, NextCursor: nextCursor, PageSize: pageSize}, nil
}

// Get returns one cache value only after a viewer user explicitly requests it.
func (service *CacheReadService) Get(ctx context.Context, key string) (CacheValue, error) {
	if service == nil || service.viewer == nil {
		return CacheValue{}, ErrCacheNotConfigured
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return CacheValue{}, ErrInvalidCacheKey
	}
	content, found, err := service.viewer.GetJSON(ctx, key)
	if err != nil {
		return CacheValue{}, err
	}
	return CacheValue{Key: key, Found: found, Content: string(content)}, nil
}

func cacheCategoryPattern(category string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "", "all":
		return "all", "*", nil
	case "trip":
		return "trip", "trip:*", nil
	case "stage":
		return "stage", "stage:*", nil
	case "busmap":
		return "busmap", "busmap:*", nil
	case "other":
		return "other", "*", nil
	default:
		return "", "", ErrInvalidCacheCategory
	}
}

func cacheKeyCategory(key string) string {
	prefix, _, found := strings.Cut(key, ":")
	if !found {
		return "other"
	}
	switch prefix {
	case "trip", "stage", "busmap":
		return prefix
	default:
		return "other"
	}
}

func cacheViewerPageSize(pageSize int64) int64 {
	if pageSize <= 0 {
		return cacheViewerDefaultPageSize
	}
	if pageSize > cacheViewerMaximumPageSize {
		return cacheViewerMaximumPageSize
	}
	return pageSize
}
