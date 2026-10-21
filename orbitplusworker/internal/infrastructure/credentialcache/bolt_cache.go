// Package credentialcache wraps an operator-credential client with a local
// BoltDB cache so repeat jobs for the same operator do not re-ask the Master.
// Entries carry a TTL; an expired entry is treated as a miss. The cache also
// supports explicit invalidation so the worker can drop a stale credential
// after a BITS auth rejection and re-ping for a fresh one.
package credentialcache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"orbitplusworker/internal/application/worker"
)

var credentialBucket = []byte("operator_credentials")

// cachedEntry is the stored shape. It is persisted to disk, so it survives a
// worker restart. The credential fields are secret and are never logged.
type cachedEntry struct {
	ZoneURL   string    `json:"zoneUrl"`
	Username  string    `json:"username"`
	APIToken  string    `json:"apiToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// BoltCache decorates an OperatorCredentialClient with an on-disk cache.
type BoltCache struct {
	delegate worker.OperatorCredentialClient
	db       *bolt.DB
	ttl      time.Duration
}

// Open opens (or creates) the Bolt database at path and returns a cache that
// serves from it, falling back to delegate on a miss or expiry. ttl bounds how
// long a cached credential is reused before a refresh ping.
func Open(path string, ttl time.Duration, delegate worker.OperatorCredentialClient) (*BoltCache, error) {
	if delegate == nil {
		return nil, fmt.Errorf("credential cache requires a delegate client")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("credential cache TTL must be positive")
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("credential cache path is required")
	}
	// Ensure the parent directory exists so a relative path like
	// "data/credentials.db" works in a fresh environment (e.g. a container
	// whose image carries only the binary). Bolt creates the file but not its
	// parent directory, so without this the open fails with "no such file or
	// directory".
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create credential cache directory %q: %w", dir, err)
		}
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open credential cache %q: %w", path, err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, bucketErr := tx.CreateBucketIfNotExists(credentialBucket)
		return bucketErr
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize credential cache: %w", err)
	}
	return &BoltCache{delegate: delegate, db: db, ttl: ttl}, nil
}

// Close releases the Bolt database.
func (cache *BoltCache) Close() error {
	if cache == nil || cache.db == nil {
		return nil
	}
	return cache.db.Close()
}

// FetchOperatorCredential returns a cached credential when present and fresh,
// otherwise pings the delegate (Master) and stores the result.
func (cache *BoltCache) FetchOperatorCredential(ctx context.Context, request worker.OperatorCredentialRequest) (worker.OperatorCredential, error) {
	operatorCode := strings.TrimSpace(request.OperatorCode)
	if operatorCode == "" {
		return worker.OperatorCredential{}, fmt.Errorf("credential request requires an operator code")
	}
	if entry, ok := cache.read(operatorCode); ok && time.Now().Before(entry.ExpiresAt) {
		slog.Info("operator credential cache hit", "operator", operatorCode)
		return worker.OperatorCredential{OperatorCode: operatorCode, ZoneURL: entry.ZoneURL, Username: entry.Username, APIToken: entry.APIToken}, nil
	}
	slog.Info("operator credential cache miss; requesting from Master", "operator", operatorCode)
	credential, err := cache.delegate.FetchOperatorCredential(ctx, request)
	if err != nil {
		return worker.OperatorCredential{}, err
	}
	cache.write(operatorCode, credential)
	return credential, nil
}

// Invalidate removes an operator's cached credential so the next fetch re-pings
// the Master. Used after a BITS auth rejection to self-heal a rotated token.
func (cache *BoltCache) Invalidate(operatorCode string) {
	operatorCode = strings.TrimSpace(operatorCode)
	if operatorCode == "" {
		return
	}
	_ = cache.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(credentialBucket)
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(operatorCode))
	})
}

func (cache *BoltCache) read(operatorCode string) (cachedEntry, bool) {
	var entry cachedEntry
	found := false
	_ = cache.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(credentialBucket)
		if bucket == nil {
			return nil
		}
		raw := bucket.Get([]byte(operatorCode))
		if raw == nil {
			return nil
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil
		}
		found = true
		return nil
	})
	return entry, found
}

func (cache *BoltCache) write(operatorCode string, credential worker.OperatorCredential) {
	entry := cachedEntry{ZoneURL: credential.ZoneURL, Username: credential.Username, APIToken: credential.APIToken, ExpiresAt: time.Now().Add(cache.ttl)}
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = cache.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(credentialBucket)
		if bucket == nil {
			return nil
		}
		return bucket.Put([]byte(operatorCode), raw)
	})
}

var _ worker.OperatorCredentialClient = (*BoltCache)(nil)
