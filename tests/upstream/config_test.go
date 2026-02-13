package upstream_test

import (
	. "ccgateway/internal/upstream"
	"testing"
)

func TestParseListEnv(t *testing.T) {
	t.Setenv("UPSTREAM_DEFAULT_ROUTE", "a, b, c ")
	got := ParseListEnv("UPSTREAM_DEFAULT_ROUTE", []string{"x"})
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("unexpected parse list result: %+v", got)
	}
}

func TestParseAdaptersFromEnv(t *testing.T) {
	t.Setenv("MY_KEY", "secret")
	t.Setenv("UPSTREAM_ADAPTERS_JSON", `[
		{
			"name":"a1",
			"kind":"openai",
			"base_url":"https://example.com",
			"api_key_env":"MY_KEY",
			"force_stream":true,
			"stream_options":{"include_usage":true}
		}
	]`)

	adapters, err := ParseAdaptersFromEnv()
	if err != nil {
		t.Fatalf("parse adapters failed: %v", err)
	}
	if len(adapters) != 1 {
		t.Fatalf("expected one adapter, got %d", len(adapters))
	}
	if adapters[0].Name() != "a1" {
		t.Fatalf("unexpected adapter name: %s", adapters[0].Name())
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("UPSTREAM_BOOL_FLAG", "true")
	if !ParseBoolEnv("UPSTREAM_BOOL_FLAG", false) {
		t.Fatalf("expected true")
	}
	t.Setenv("UPSTREAM_BOOL_FLAG", "no")
	if ParseBoolEnv("UPSTREAM_BOOL_FLAG", true) {
		t.Fatalf("expected false")
	}
}
