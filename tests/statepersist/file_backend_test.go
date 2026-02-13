package statepersist_test

import (
	. "ccgateway/internal/statepersist"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileBackendSaveLoad(t *testing.T) {
	dir := t.TempDir()
	backend, err := NewFileBackend(dir)
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}

	payload := map[string]any{"a": "b", "n": 1}
	if err := backend.Save("runs", payload); err != nil {
		t.Fatalf("save: %v", err)
	}

	var out map[string]any
	if err := backend.Load("runs", &out); err != nil {
		t.Fatalf("load: %v", err)
	}
	if out["a"] != "b" {
		t.Fatalf("unexpected payload: %#v", out)
	}

	path := filepath.Join(dir, "runs.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted file: %v", err)
	}
}

func TestFileBackendNotFound(t *testing.T) {
	backend, err := NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	var out map[string]any
	err = backend.Load("missing", &out)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileBackendKeyValidation(t *testing.T) {
	backend, err := NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	if err := backend.Save("bad/key", map[string]any{}); err == nil {
		t.Fatalf("expected key validation error")
	}
	if err := backend.Load("", &map[string]any{}); err == nil {
		t.Fatalf("expected empty key validation error")
	}
}
