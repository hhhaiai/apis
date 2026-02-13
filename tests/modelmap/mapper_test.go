package modelmap_test

import (
	. "ccgateway/internal/modelmap"
	"testing"
)

func TestStaticMapperResolveMapped(t *testing.T) {
	m := NewStaticMapper(map[string]string{
		"a": "b",
	}, true, "")
	got, err := m.Resolve("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b" {
		t.Fatalf("expected b, got %q", got)
	}
}

func TestStaticMapperStrictRejectUnknown(t *testing.T) {
	m := NewStaticMapper(map[string]string{
		"a": "b",
	}, true, "")
	_, err := m.Resolve("unknown")
	if err == nil {
		t.Fatalf("expected strict mode error")
	}
}

func TestStaticMapperFallback(t *testing.T) {
	m := NewStaticMapper(map[string]string{
		"a": "b",
	}, false, "fallback")
	got, err := m.Resolve("unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestStaticMapperWildcard(t *testing.T) {
	m := NewStaticMapper(map[string]string{
		"claude-*": "gemini-3-pro-high",
	}, true, "")
	got, err := m.Resolve("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "gemini-3-pro-high" {
		t.Fatalf("expected wildcard mapped target, got %q", got)
	}
}
