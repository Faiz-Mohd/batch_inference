package handler

import (
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/example/batch-inference/internal/logging"
)

// MockConfig configures the mock inference endpoint's behavior.
type MockConfig struct {
	RatePerSec float64
	Burst      int
	FailRate   float64
	MaxLatency time.Duration
}

// MockHandler simulates a rate-limited inference API: it enforces a token-bucket
// limit (429 + Retry-After when exceeded) and can inject random latency/errors
// to exercise the retry path.
type MockHandler struct {
	limiter    *rate.Limiter
	failRate   float64
	maxLatency time.Duration
}

// NewMockHandler constructs a MockHandler.
func NewMockHandler(cfg MockConfig) *MockHandler {
	burst := cfg.Burst
	if burst < 1 {
		burst = 1
	}
	return &MockHandler{
		limiter:    rate.NewLimiter(rate.Limit(cfg.RatePerSec), burst),
		failRate:   cfg.FailRate,
		maxLatency: cfg.MaxLatency,
	}
}

// Infer handles POST /mock/infer.
func (h *MockHandler) Infer(c *gin.Context) {
	ctx := c.Request.Context()
	logger := logging.FromContext(ctx)

	// Enforce the rate limit. If no token is available now, tell the caller how
	// long to wait via Retry-After, mirroring a real rate-limited API.
	reservation := h.limiter.Reserve()
	if delay := reservation.Delay(); delay > 0 {
		reservation.Cancel()
		seconds := int(math.Ceil(delay.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		c.Header("Retry-After", strconv.Itoa(seconds))
		logger.Debug("mock rate limited", "retry_after_s", seconds)
		c.JSON(http.StatusTooManyRequests, errorResponse{Error: "rate limit exceeded"})
		return
	}

	var req inferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	// Simulate variable processing latency.
	if h.maxLatency > 0 {
		time.Sleep(time.Duration(rand.Int64N(int64(h.maxLatency) + 1)))
	}

	// Simulate occasional transient server errors.
	if h.failRate > 0 && rand.Float64() < h.failRate {
		logger.Debug("mock injected failure")
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "simulated upstream error"})
		return
	}

	c.JSON(http.StatusOK, inferResponse{
		Completion: fmt.Sprintf("mock completion for prompt of %d chars", len(req.Prompt)),
	})
}
