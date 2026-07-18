package domain

import (
	"errors"
	"fmt"
	"time"
)

// Validation errors returned by the batch service. Handlers map these to 400.
var (
	ErrEmptyBatch    = errors.New("batch must contain at least one prompt")
	ErrBatchTooLarge = errors.New("batch exceeds the maximum allowed size")
	ErrEmptyPrompt   = errors.New("prompt text must not be empty")
	ErrPromptTooLong = errors.New("prompt exceeds the maximum allowed length")
)

// RetryableError marks a transient failure that should be retried with backoff.
// RetryAfter carries a server-suggested delay (e.g. from a 429 Retry-After
// header); zero means "no suggestion, use computed backoff".
//
// RateLimited distinguishes throttling (HTTP 429) from genuine transient errors
// (5xx / transport). Throttling is not the prompt's fault, so it must not count
// against the retry budget -- otherwise sustained rate limiting would drop
// prompts once attempts are exhausted.
type RetryableError struct {
	Err         error
	RetryAfter  time.Duration
	RateLimited bool
}

func (e *RetryableError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("retryable: %v (retry-after=%s)", e.Err, e.RetryAfter)
	}
	return fmt.Sprintf("retryable: %v", e.Err)
}

func (e *RetryableError) Unwrap() error { return e.Err }

// NewRetryable wraps err as a genuine transient failure (5xx / transport) that
// counts against the retry budget.
func NewRetryable(err error, retryAfter time.Duration) *RetryableError {
	return &RetryableError{Err: err, RetryAfter: retryAfter}
}

// NewRateLimited wraps err as a throttling (429) failure. It is retried without
// consuming the attempt budget so prompts are never dropped due to rate limits.
func NewRateLimited(err error, retryAfter time.Duration) *RetryableError {
	return &RetryableError{Err: err, RetryAfter: retryAfter, RateLimited: true}
}

// AsRetryable reports whether err (or anything it wraps) is a RetryableError.
func AsRetryable(err error) (*RetryableError, bool) {
	var re *RetryableError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}
