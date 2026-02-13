package ratelimit_test

import (
	. "ccgateway/internal/ratelimit"
	"testing"
)

func TestLimiter_AllowWithinBurst(t *testing.T) {
	l := New(10, 5)
	for i := 0; i < 5; i++ {
		if !l.Allow("key1") {
			t.Fatalf("request %d should be allowed within burst", i)
		}
	}
}

func TestLimiter_DenyAfterBurstExhausted(t *testing.T) {
	l := New(1, 2) // 1 rps, burst 2
	l.Allow("key1")
	l.Allow("key1")
	if l.Allow("key1") {
		t.Fatal("third request should be denied after burst exhausted")
	}
}

func TestLimiter_KeyIsolation(t *testing.T) {
	l := New(1, 1)
	if !l.Allow("key_a") {
		t.Fatal("key_a should be allowed")
	}
	if !l.Allow("key_b") {
		t.Fatal("key_b should be allowed (separate bucket)")
	}
	if l.Allow("key_a") {
		t.Fatal("key_a second request should be denied")
	}
}

func TestLimiter_DefaultValues(t *testing.T) {
	l := New(0, 0) // should default to 100 rps
	if l.TestGetRPS() != 100 {
		t.Fatalf("expected default 100 rps, got %f", l.TestGetRPS())
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	l := New(10, 5)
	l.Allow("stale_key")
	l.TestCleanup(0) // maxAge=0 means remove everything
	count := l.TestBucketCount()
	if count != 0 {
		t.Fatalf("expected 0 buckets after cleanup, got %d", count)
	}
}
