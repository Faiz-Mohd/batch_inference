package service

import (
	"testing"
	"time"
)

func TestBackoffHonorsRetryAfter(t *testing.T) {
	base, max := 100*time.Millisecond, 10*time.Second

	if got := backoff(1, base, max, 2*time.Second); got != 2*time.Second {
		t.Errorf("backoff = %v, want Retry-After of 2s", got)
	}
	// Retry-After beyond the ceiling is capped.
	if got := backoff(1, base, max, time.Minute); got != max {
		t.Errorf("backoff = %v, want capped at %v", got, max)
	}
}

func TestBackoffJitterBounds(t *testing.T) {
	base, max := 100*time.Millisecond, 10*time.Second

	for attempt := 0; attempt <= 40; attempt++ {
		for i := 0; i < 50; i++ {
			got := backoff(attempt, base, max, 0)
			if got < base {
				t.Fatalf("attempt %d: backoff %v below base %v", attempt, got, base)
			}
			if got > max {
				t.Fatalf("attempt %d: backoff %v above max %v", attempt, got, max)
			}
		}
	}
}

func TestBackoffGrowsWithAttempts(t *testing.T) {
	base, max := 100*time.Millisecond, 10*time.Second

	// The exponential ceiling for attempt 1 is exactly base, so the delay is
	// deterministic; later attempts may jitter anywhere in [base, ceiling].
	if got := backoff(1, base, max, 0); got != base {
		t.Errorf("attempt 1: backoff = %v, want %v", got, base)
	}

	ceiling := base << 3 // attempt 4
	for i := 0; i < 50; i++ {
		if got := backoff(4, base, max, 0); got > ceiling {
			t.Fatalf("attempt 4: backoff %v above ceiling %v", got, ceiling)
		}
	}
}
