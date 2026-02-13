package statepersist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileBackend struct {
	mu  sync.Mutex
	dir string
}

func NewFileBackend(dir string) (*FileBackend, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("persist directory is required")
	}
	clean := filepath.Clean(dir)
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return nil, fmt.Errorf("create persist dir: %w", err)
	}
	return &FileBackend{dir: clean}, nil
}

func (b *FileBackend) Load(key string, out any) error {
	name, err := normalizeKey(key)
	if err != nil {
		return err
	}
	path := filepath.Join(b.dir, name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(raw, out)
}

func (b *FileBackend) Save(key string, value any) error {
	name, err := normalizeKey(key)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	path := filepath.Join(b.dir, name+".json")
	tmp := path + ".tmp"
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normalizeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("persist key is required")
	}
	if strings.Contains(key, "/") || strings.Contains(key, "\\") {
		return "", fmt.Errorf("persist key contains invalid path separator")
	}
	return key, nil
}
