package service

import (
	"math/rand"
	"time"
)

// backoff computes the delay before the next attempt. If the server suggested a
// delay (Retry-After), it is honored (capped at max). Otherwise it uses
// exponential backoff with full jitter to avoid synchronized retry storms:
//
//	delay = random(0, min(max, base * 2^(attempt-1)))
func backoff(attempt int, base, max, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > max {
			return max
		}
		return retryAfter
	}

	if attempt < 1 {
		attempt = 1
	}

	// Cap the shift to avoid overflow, then clamp to max.
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	ceiling := base << uint(shift)
	if ceiling <= 0 || ceiling > max {
		ceiling = max
	}

	// Full jitter in [base, ceiling] so we never retry with zero delay.
	if ceiling <= base {
		return base
	}
	return base + time.Duration(rand.Int63n(int64(ceiling-base)))
}
