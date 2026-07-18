package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
	"github.com/example/batch-inference/internal/logging"
)

// BatchService is the service contract the batch handler depends on.
type BatchService interface {
	CreateBatch(ctx context.Context, prompts []string) (*domain.Batch, error)
	GetBatch(ctx context.Context, id uuid.UUID) (*domain.Batch, domain.BatchCounts, error)
}

// BatchHandler handles batch submission and status lookups.
type BatchHandler struct {
	service      BatchService
	maxBodyBytes int64
}

// NewBatchHandler constructs a BatchHandler.
func NewBatchHandler(service BatchService, maxBodyBytes int64) *BatchHandler {
	return &BatchHandler{service: service, maxBodyBytes: maxBodyBytes}
}

// Create validates the incoming prompts and returns an immediate 202 ack with a
// batch ID. The request body must be a JSON array of prompt strings, e.g.
// ["prompt one", "prompt two"].
func (h *BatchHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	logger := logging.FromContext(ctx)

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBodyBytes)

	var req createBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		c.JSON(http.StatusBadRequest, errorResponse{Error: "request body must be a JSON array of prompts"})
		return
	}

	batch, err := h.service.CreateBatch(ctx, req)
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

// Get returns a batch's status, live prompt counts, and (once completed) its
// aggregated result.
func (h *BatchHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{Error: "batch id must be a valid UUID"})
		return
	}

	batch, counts, err := h.service.GetBatch(ctx, id)
	if err != nil {
		logging.FromContext(ctx).Error("failed to get batch", "err", err, "batch_id", id)
		c.JSON(http.StatusInternalServerError, errorResponse{Error: "failed to get batch"})
		return
	}
	if batch == nil {
		c.JSON(http.StatusNotFound, errorResponse{Error: "batch not found"})
		return
	}

	c.JSON(http.StatusOK, batchStatusResponse{
		BatchID:     batch.ID,
		Status:      string(batch.Status),
		Total:       counts.Total,
		Succeeded:   counts.Succeeded,
		Failed:      counts.Failed,
		CreatedAt:   batch.CreatedAt,
		CompletedAt: batch.CompletedAt,
		Result:      batch.Result,
	})
}

func isValidationError(err error) bool {
	return errors.Is(err, domain.ErrEmptyBatch) ||
		errors.Is(err, domain.ErrBatchTooLarge) ||
		errors.Is(err, domain.ErrEmptyPrompt) ||
		errors.Is(err, domain.ErrPromptTooLong)
}
