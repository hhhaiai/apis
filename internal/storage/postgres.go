package storage

import (
	"context"
	"fmt"
	"strings"
)

// PostgresConfig holds PostgreSQL connection details.
type PostgresConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
}

// DSN returns the connection string.
func (c PostgresConfig) DSN() string {
	ssl := c.SSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, ssl)
}

// PostgresBackend implements Backend using PostgreSQL.
// It wraps an in-memory map and is ready for integration with a real driver
// (e.g. pgx or database/sql) by replacing the internal methods.
type PostgresBackend struct {
	config PostgresConfig
	mem    *MemoryBackend // in-memory fallback; swap with real driver
}

// NewPostgresBackend creates a Postgres backend.
// Currently uses in-memory storage but provides the connection config
// for integration with a real PostgreSQL driver (pgx, lib/pq).
func NewPostgresBackend(cfg PostgresConfig) *PostgresBackend {
	return &PostgresBackend{
		config: cfg,
		mem:    NewMemoryBackend(),
	}
}

func (p *PostgresBackend) Get(ctx context.Context, key string) (string, bool, error) {
	return p.mem.Get(ctx, key)
}

func (p *PostgresBackend) Set(ctx context.Context, key, value string) error {
	return p.mem.Set(ctx, key, value)
}

func (p *PostgresBackend) Delete(ctx context.Context, key string) error {
	return p.mem.Delete(ctx, key)
}

func (p *PostgresBackend) List(ctx context.Context, prefix string) ([]string, error) {
	return p.mem.List(ctx, prefix)
}

func (p *PostgresBackend) Close() error {
	return nil
}

// Config returns the PostgreSQL configuration.
func (p *PostgresBackend) Config() PostgresConfig {
	return p.config
}

// CreateTable returns the SQL to create the KV table.
func (p *PostgresBackend) CreateTable() string {
	return strings.TrimSpace(`
CREATE TABLE IF NOT EXISTS cc_kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cc_kv_prefix ON cc_kv (key text_pattern_ops);
`)
}
