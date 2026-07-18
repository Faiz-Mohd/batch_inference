package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/batch-inference/internal/domain"
)

// PromptRepository provides prompt-level persistence and the durable-queue
// claim/update operations.
type PromptRepository struct {
	pool *pgxpool.Pool
}

// NewPromptRepository constructs a PromptRepository.
func NewPromptRepository(pool *pgxpool.Pool) *PromptRepository {
	return &PromptRepository{pool: pool}
}

// Claim atomically leases up to limit due prompts, transitioning them to
// 'processing' and incrementing attempts. FOR UPDATE SKIP LOCKED guarantees no
// two workers (or instances) claim the same row.
func (r *PromptRepository) Claim(ctx context.Context, limit int) ([]domain.Prompt, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE prompts
		SET status = 'processing', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM prompts
			WHERE status = 'pending' AND next_retry_at <= now()
			ORDER BY next_retry_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id, batch_id, seq, prompt_text, attempts, max_attempts`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("claim prompts: %w", err)
	}
	defer rows.Close()

	var out []domain.Prompt
	for rows.Next() {
		var p domain.Prompt
		if err := rows.Scan(&p.ID, &p.BatchID, &p.Seq, &p.Text, &p.Attempts, &p.MaxAttempts); err != nil {
			return nil, fmt.Errorf("scan claimed prompt: %w", err)
		}
		p.Status = domain.PromptProcessing
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed prompts: %w", err)
	}
	return out, nil
}

// Succeed marks a prompt as succeeded and stores its response.
func (r *PromptRepository) Succeed(ctx context.Context, id uuid.UUID, response string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prompts
		SET status = 'succeeded', response = $2, error = NULL, updated_at = now()
		WHERE id = $1`,
		id, response,
	)
	if err != nil {
		return fmt.Errorf("mark succeeded: %w", err)
	}
	return nil
}

// Requeue reschedules a prompt back to 'pending' with a future next_retry_at.
// The durable timestamp is how backoff survives restarts.
//
// When consumeAttempt is false (rate-limited / 429), attempts is decremented to
// cancel out the increment applied when the prompt was claimed, so throttling
// never eats the retry budget and never drops a prompt.
func (r *PromptRepository) Requeue(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, lastErr string, consumeAttempt bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prompts
		SET status = 'pending',
		    next_retry_at = $2,
		    error = $3,
		    attempts = CASE WHEN $4 THEN attempts ELSE GREATEST(attempts - 1, 0) END,
		    updated_at = now()
		WHERE id = $1`,
		id, nextRetryAt, lastErr, consumeAttempt,
	)
	if err != nil {
		return fmt.Errorf("requeue prompt: %w", err)
	}
	return nil
}

// Fail marks a prompt as permanently failed.
func (r *PromptRepository) Fail(ctx context.Context, id uuid.UUID, lastErr string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prompts
		SET status = 'failed', error = $2, updated_at = now()
		WHERE id = $1`,
		id, lastErr,
	)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

// RecoverStuck resets prompts left in 'processing' (e.g. by a crash) back to
// 'pending' so they are reclaimed. Returns the number of rows recovered.
func (r *PromptRepository) RecoverStuck(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE prompts
		SET status = 'pending', updated_at = now()
		WHERE status = 'processing'`,
	)
	if err != nil {
		return 0, fmt.Errorf("recover stuck prompts: %w", err)
	}
	return tag.RowsAffected(), nil
}
