package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/example/batch-inference/internal/logging"
)

// Pinger reports database connectivity.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves liveness/readiness checks.
type HealthHandler struct {
	pinger Pinger
}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler(pinger Pinger) *HealthHandler {
	return &HealthHandler{pinger: pinger}
}

// Check returns 200 when the database is reachable, 503 otherwise.
func (h *HealthHandler) Check(c *gin.Context) {
	ctx := c.Request.Context()
	if err := h.pinger.Ping(ctx); err != nil {
		logging.FromContext(ctx).Error("health check failed", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
