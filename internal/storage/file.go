package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileBackend persists data as JSON files in a directory.
type FileBackend struct {
	mu  sync.RWMutex
	dir string
}

// NewFileBackend creates a file-based backend.
func NewFileBackend(dir string) (*FileBackend, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}
	return &FileBackend{dir: dir}, nil
}

func (f *FileBackend) keyPath(key string) string {
	// Replace / in key with __
	safe := strings.ReplaceAll(key, "/", "__")
	return filepath.Join(f.dir, safe+".json")
}

func (f *FileBackend) Get(_ context.Context, key string) (string, bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, err := os.ReadFile(f.keyPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data), true, nil // raw string fallback
	}
	return value, true, nil
}

func (f *FileBackend) Set(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, _ := json.Marshal(value)
	return os.WriteFile(f.keyPath(key), data, 0644)
}

func (f *FileBackend) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := os.Remove(f.keyPath(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (f *FileBackend) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		key := strings.ReplaceAll(name, "__", "/")
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (f *FileBackend) Close() error {
	return nil
}
