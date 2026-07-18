// Package service holds the business logic: request validation, batch creation,
// the worker pool, retry/backoff policy, and result aggregation. It depends only
// on repository and inference-client interfaces (no gin, no SQL).
package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
	"github.com/example/batch-inference/internal/logging"
)

// BatchService validates and persists incoming batches (the immediate-ack path).
type BatchService struct {
	batches      BatchRepository
	maxBatchSize int
	maxPromptLen int
	maxAttempts  int
}

// NewBatchService constructs a BatchService.
func NewBatchService(batches BatchRepository, maxBatchSize, maxPromptLen, maxAttempts int) *BatchService {
	return &BatchService{
		batches:      batches,
		maxBatchSize: maxBatchSize,
		maxPromptLen: maxPromptLen,
		maxAttempts:  maxAttempts,
	}
}

// CreateBatch validates the prompts and persists the batch plus its prompt rows.
// It returns quickly; no inference work happens here.
func (s *BatchService) CreateBatch(ctx context.Context, prompts []string) (*domain.Batch, error) {
	cleaned, err := s.validate(prompts)
	if err != nil {
		return nil, err
	}

	batch := &domain.Batch{
		ID:         uuid.New(),
		Status:     domain.BatchPending,
		TotalCount: len(cleaned),
	}

	if err := s.batches.Create(ctx, batch, cleaned, s.maxAttempts); err != nil {
		return nil, err
	}

	logging.FromContext(ctx).Info("batch created",
		"batch_id", batch.ID,
		"prompt_count", batch.TotalCount,
	)
	return batch, nil
}

// GetBatch returns a batch with its live prompt counts. A nil batch (and nil
// error) means the ID is unknown.
func (s *BatchService) GetBatch(ctx context.Context, id uuid.UUID) (*domain.Batch, domain.BatchCounts, error) {
	batch, err := s.batches.Get(ctx, id)
	if err != nil || batch == nil {
		return nil, domain.BatchCounts{}, err
	}
	counts, err := s.batches.Counts(ctx, id)
	if err != nil {
		return nil, domain.BatchCounts{}, err
	}
	return batch, counts, nil
}

func (s *BatchService) validate(prompts []string) ([]string, error) {
	if len(prompts) == 0 {
		return nil, domain.ErrEmptyBatch
	}
	if len(prompts) > s.maxBatchSize {
		return nil, domain.ErrBatchTooLarge
	}

	cleaned := make([]string, len(prompts))
	for i, p := range prompts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return nil, domain.ErrEmptyPrompt
		}
		if len(trimmed) > s.maxPromptLen {
			return nil, domain.ErrPromptTooLong
		}
		cleaned[i] = trimmed
	}
	return cleaned, nil
}
