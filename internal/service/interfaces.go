package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

// BatchRepository is the batch-level persistence contract the service depends
// on. The postgres package provides the concrete implementation.
type BatchRepository interface {
	Create(ctx context.Context, batch *domain.Batch, prompts []string, maxAttempts int) error
	Get(ctx context.Context, batchID uuid.UUID) (*domain.Batch, error)
	Counts(ctx context.Context, batchID uuid.UUID) (domain.BatchCounts, error)
	GetSucceeded(ctx context.Context, batchID uuid.UUID) ([]domain.Prompt, error)
	Complete(ctx context.Context, batchID uuid.UUID, result []byte) (bool, error)
}

// PromptRepository is the durable-queue persistence contract.
type PromptRepository interface {
	Claim(ctx context.Context, limit int) ([]domain.Prompt, error)
	Succeed(ctx context.Context, id uuid.UUID, response string) error
	Requeue(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, lastErr string, consumeAttempt bool) error
	Fail(ctx context.Context, id uuid.UUID, lastErr string) error
	RecoverStuck(ctx context.Context) (int64, error)
}

// InferenceClient calls the downstream inference endpoint.
type InferenceClient interface {
	Infer(ctx context.Context, prompt string) (string, error)
}
