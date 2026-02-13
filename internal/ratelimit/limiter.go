package ratelimit

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter implements a per-key token bucket rate limiter.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   int
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// New creates a new rate limiter with the given requests-per-second and burst size.
func New(rps float64, burst int) *Limiter {
	if rps <= 0 {
		rps = 100
	}
	if burst <= 0 {
		burst = int(rps)
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		rps:     rps,
		burst:   burst,
	}
}

// NewFromEnv creates a Limiter from environment variables:
//   - RATE_LIMIT_RPS: requests per second (default 100)
//   - RATE_LIMIT_BURST: burst size (default same as rps)
func NewFromEnv() *Limiter {
	rps := parseFloatEnv("RATE_LIMIT_RPS", 100)
	burst := parseIntEnv("RATE_LIMIT_BURST", int(rps))
	return New(rps, burst)
}

// Allow checks if a request from the given key is allowed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{
			tokens:    float64(l.burst) - 1,
			lastCheck: now,
		}
		l.buckets[key] = b
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * l.rps
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Cleanup removes stale buckets that haven't been accessed recently.
func (l *Limiter) Cleanup(maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for key, b := range l.buckets {
		if b.lastCheck.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

func parseFloatEnv(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func parseIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}
