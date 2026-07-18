package handler

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/example/batch-inference/internal/handler/middleware"
)

// Router wires the gin engine with middleware and routes.
type Router struct {
	Batch  *BatchHandler
	Health *HealthHandler
	Mock   *MockHandler
	Logger *slog.Logger
}

// Build constructs the gin engine. The mock endpoint is registered only when a
// MockHandler is provided.
func (r Router) Build() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.Logger(r.Logger),
		middleware.Recovery(r.Logger),
	)

	engine.GET("/healthz", r.Health.Check)

	v1 := engine.Group("/v1")
	v1.POST("/batches", r.Batch.Create)
	v1.GET("/batches/:id", r.Batch.Get)

	if r.Mock != nil {
		engine.POST("/mock/infer", r.Mock.Infer)
	}

	return engine
}
