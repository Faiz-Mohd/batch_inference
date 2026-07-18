package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/example/batch-inference/internal/domain"
	"github.com/example/batch-inference/internal/logging"
)

// BatchCreator is the service contract the batch handler depends on.
type BatchCreator interface {
	CreateBatch(ctx context.Context, prompts []string) (*domain.Batch, error)
}

// BatchHandler handles batch submission.
type BatchHandler struct {
	service      BatchCreator
	maxBodyBytes int64
}

// NewBatchHandler constructs a BatchHandler.
func NewBatchHandler(service BatchCreator, maxBodyBytes int64) *BatchHandler {
	return &BatchHandler{service: service, maxBodyBytes: maxBodyBytes}
}

// Create validates the incoming prompts and returns an immediate 202 ack with a
// batch ID. The request body must be a JSON array of prompt strings, e.g.
// ["prompt one", "prompt two"].
func (h *BatchHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	logger := logging.FromContext(ctx)

	var req createBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "request body must be a JSON array of prompts"})
		return
	}

	batch, err := h.service.CreateBatch(ctx, req.Prompts())
	if err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		logger.Error("failed to create batch", "err", err)
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "failed to create batch"})
		return
	}

	c.JSON(http.StatusAccepted, createBatchResponse{
		BatchID:  batch.ID,
		Accepted: batch.TotalCount,
		Status:   string(batch.Status),
	})
}

func isValidationError(err error) bool {
	return errors.Is(err, domain.ErrEmptyBatch) ||
		errors.Is(err, domain.ErrBatchTooLarge) ||
		errors.Is(err, domain.ErrEmptyPrompt) ||
		errors.Is(err, domain.ErrPromptTooLong)
}
