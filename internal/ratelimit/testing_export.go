package ratelimit

import "time"

// Export unexported fields for external tests.

// TestGetRPS returns the limiter's rps for testing.
func (l *Limiter) TestGetRPS() float64 {
	return l.rps
}

// TestBucketCount returns the number of buckets for testing.
func (l *Limiter) TestBucketCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// TestCleanup calls Cleanup for testing (wraps to avoid import issues).
func (l *Limiter) TestCleanup(maxAge time.Duration) {
	l.Cleanup(maxAge)
}
