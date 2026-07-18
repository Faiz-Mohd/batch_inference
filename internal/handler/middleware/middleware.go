// Package middleware provides gin middleware for request IDs, structured access
// logging, and panic recovery. A request-scoped logger is injected into the
// request context so downstream layers emit correlated logs.
package middleware

import (
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/logging"
)

const requestIDHeader = "X-Request-ID"

// RequestID ensures every request has a correlation ID, echoed in the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Writer.Header().Set(requestIDHeader, id)
		c.Set("request_id", id)
		c.Next()
	}
}

// Logger injects a request-scoped slog.Logger into the context and emits one
// structured access log per request.
func Logger(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestID, _ := c.Get("request_id")
		reqLogger := base.With(
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
		)

		ctx := logging.WithContext(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"client_ip", c.ClientIP(),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}

		switch {
		case status >= 500:
			reqLogger.Error("request completed", attrs...)
		case status >= 400:
			reqLogger.Warn("request completed", attrs...)
		default:
			reqLogger.Info("request completed", attrs...)
		}
	}
}

// Recovery logs panics with a stack trace and returns a 500 instead of crashing.
func Recovery(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.FromContext(c.Request.Context()).Error("panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
				}
			}
		}()
		c.Next()
	}
}
