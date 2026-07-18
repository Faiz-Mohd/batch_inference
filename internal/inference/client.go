// Package inference provides an HTTP client for the (mock or real) rate-limited
// inference endpoint. Transient failures are surfaced as domain.RetryableError
// so the processor can decide whether to requeue with backoff.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/example/batch-inference/internal/domain"
)

// Request is the payload sent to the inference endpoint.
type Request struct {
	Prompt string `json:"prompt"`
}

// Response is the successful payload returned by the inference endpoint.
type Response struct {
	Completion string `json:"completion"`
}

// Client calls the inference endpoint over HTTP.
type Client struct {
	url        string
	httpClient *http.Client
}

// New creates a Client targeting url with the given per-request timeout.
func New(url string, timeout time.Duration) *Client {
	return &Client{
		url: url,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Infer sends prompt to the endpoint and returns the completion text.
// On HTTP 429 or 5xx (or transport errors) it returns a *domain.RetryableError.
func (c *Client) Infer(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(Request{Prompt: prompt})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Transport-level errors (timeouts, connection refused) are transient.
		return "", domain.NewRetryable(fmt.Errorf("inference request failed: %w", err), 0)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusOK:
		var out Response
		if err := json.Unmarshal(respBody, &out); err != nil {
			return "", fmt.Errorf("decode response: %w", err)
		}
		return out.Completion, nil

	case resp.StatusCode == http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return "", domain.NewRateLimited(
			fmt.Errorf("rate limited (429): %s", truncate(string(respBody), 200)),
			retryAfter,
		)

	case resp.StatusCode >= 500:
		return "", domain.NewRetryable(
			fmt.Errorf("server error (%d): %s", resp.StatusCode, truncate(string(respBody), 200)),
			0,
		)

	default:
		// 4xx (other than 429) are permanent: retrying will not help.
		return "", fmt.Errorf("inference failed (%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
}

// parseRetryAfter parses a Retry-After header expressed either as delta-seconds
// or an HTTP date. Returns 0 when absent or unparseable.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
