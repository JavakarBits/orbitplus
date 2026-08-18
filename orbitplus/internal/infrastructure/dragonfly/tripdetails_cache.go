package dragonfly

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config contains the Dragonfly connection settings supplied by runtime config.
type Config struct {
	Address     string
	Password    string
	Database    int
	DialTimeout time.Duration
}

// TripDetailsCacheRepository stores JSON documents in Dragonfly.
type TripDetailsCacheRepository struct {
	client *redis.Client
}

func NewTripDetailsCacheRepository(ctx context.Context, config Config) (*TripDetailsCacheRepository, error) {
	client := redis.NewClient(&redis.Options{
		Addr: config.Address, Password: config.Password, DB: config.Database, DialTimeout: config.DialTimeout,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Dragonfly: %w", err)
	}
	return &TripDetailsCacheRepository{client: client}, nil
}

// SetJSON stores one JSON document without an expiry policy.
func (repository *TripDetailsCacheRepository) SetJSON(ctx context.Context, key string, value []byte) error {
	if err := repository.client.Set(ctx, key, value, 0).Err(); err != nil {
		return fmt.Errorf("set Dragonfly key %q: %w", key, err)
	}
	return nil
}

// GetJSON retrieves one JSON document and distinguishes a cache miss from an error.
func (repository *TripDetailsCacheRepository) GetJSON(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := repository.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get Dragonfly key %q: %w", key, err)
	}
	return value, true, nil
}

func (repository *TripDetailsCacheRepository) Close() error {
	return repository.client.Close()
}
