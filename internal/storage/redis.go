package storage

import (
	"context"
	"fmt"
)

// RedisConfig holds Redis connection details.
type RedisConfig struct {
	Addr     string `json:"addr"` // host:port
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// RedisBackend implements Backend using Redis.
// It wraps an in-memory map and is ready for integration with a real driver
// (e.g. go-redis) by replacing the internal methods.
type RedisBackend struct {
	config RedisConfig
	mem    *MemoryBackend // in-memory fallback; swap with real driver
}

// NewRedisBackend creates a Redis backend.
// Currently uses in-memory storage but provides the connection config
// for integration with a real Redis driver (go-redis/redis).
func NewRedisBackend(cfg RedisConfig) *RedisBackend {
	return &RedisBackend{
		config: cfg,
		mem:    NewMemoryBackend(),
	}
}

func (r *RedisBackend) Get(ctx context.Context, key string) (string, bool, error) {
	return r.mem.Get(ctx, key)
}

func (r *RedisBackend) Set(ctx context.Context, key, value string) error {
	return r.mem.Set(ctx, key, value)
}

func (r *RedisBackend) Delete(ctx context.Context, key string) error {
	return r.mem.Delete(ctx, key)
}

func (r *RedisBackend) List(ctx context.Context, prefix string) ([]string, error) {
	return r.mem.List(ctx, prefix)
}

func (r *RedisBackend) Close() error {
	return nil
}

// Config returns the Redis configuration.
func (r *RedisBackend) Config() RedisConfig {
	return r.config
}

// ConnectionString returns the Redis connection URL.
func (r *RedisBackend) ConnectionString() string {
	if r.config.Password != "" {
		return fmt.Sprintf("redis://:%s@%s/%d", r.config.Password, r.config.Addr, r.config.DB)
	}
	return fmt.Sprintf("redis://%s/%d", r.config.Addr, r.config.DB)
}
