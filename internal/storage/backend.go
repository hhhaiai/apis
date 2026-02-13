package storage

import (
	"context"
	"fmt"
)

// Backend provides a unified storage interface for all persistent data.
type Backend interface {
	// Get retrieves a value by key. Returns empty string and false if not found.
	Get(ctx context.Context, key string) (string, bool, error)
	// Set stores a value by key.
	Set(ctx context.Context, key, value string) error
	// Delete removes a key.
	Delete(ctx context.Context, key string) error
	// List returns all keys matching the given prefix.
	List(ctx context.Context, prefix string) ([]string, error)
	// Close releases any resources.
	Close() error
}

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = fmt.Errorf("key not found")
