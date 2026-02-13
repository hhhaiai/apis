package storage_test

import (
	. "ccgateway/internal/storage"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// testBackend runs the standard test suite against any Backend implementation.
func testBackend(t *testing.T, b Backend) {
	t.Helper()
	ctx := context.Background()

	// Set and Get
	if err := b.Set(ctx, "key1", "value1"); err != nil {
		t.Fatal("Set failed:", err)
	}
	v, ok, err := b.Get(ctx, "key1")
	if err != nil {
		t.Fatal("Get failed:", err)
	}
	if !ok || v != "value1" {
		t.Fatalf("expected value1, got %q (ok=%v)", v, ok)
	}

	// Get non-existent
	_, ok, err = b.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatal("Get nonexistent failed:", err)
	}
	if ok {
		t.Fatal("expected not found")
	}

	// List with prefix
	_ = b.Set(ctx, "prefix/a", "va")
	_ = b.Set(ctx, "prefix/b", "vb")
	_ = b.Set(ctx, "other/c", "vc")
	keys, err := b.List(ctx, "prefix/")
	if err != nil {
		t.Fatal("List failed:", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys with prefix, got %d", len(keys))
	}

	// Delete
	if err := b.Delete(ctx, "key1"); err != nil {
		t.Fatal("Delete failed:", err)
	}
	_, ok, _ = b.Get(ctx, "key1")
	if ok {
		t.Fatal("key1 should be deleted")
	}

	// Close
	if err := b.Close(); err != nil {
		t.Fatal("Close failed:", err)
	}
}

func TestMemoryBackend(t *testing.T) {
	testBackend(t, NewMemoryBackend())
}

func TestFileBackend(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "ccgateway_test_file_backend")
	defer os.RemoveAll(dir)
	b, err := NewFileBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	testBackend(t, b)
}

func TestPostgresBackend(t *testing.T) {
	b := NewPostgresBackend(PostgresConfig{
		Host: "localhost", Port: 5432, User: "test", Database: "test",
	})
	testBackend(t, b)

	// Test DSN
	dsn := b.Config().DSN()
	if dsn == "" {
		t.Fatal("empty DSN")
	}

	// Test CreateTable SQL
	sql := b.CreateTable()
	if sql == "" {
		t.Fatal("empty create table SQL")
	}
}

func TestRedisBackend(t *testing.T) {
	b := NewRedisBackend(RedisConfig{Addr: "localhost:6379", DB: 0})
	testBackend(t, b)

	// Test connection string
	cs := b.ConnectionString()
	if cs == "" {
		t.Fatal("empty connection string")
	}
}

func TestRedisBackend_WithPassword(t *testing.T) {
	b := NewRedisBackend(RedisConfig{Addr: "localhost:6379", Password: "secret", DB: 1})
	cs := b.ConnectionString()
	if cs != "redis://:secret@localhost:6379/1" {
		t.Fatalf("unexpected connection string: %s", cs)
	}
}
