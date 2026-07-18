package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/batch-inference/internal/domain"
)

// BatchRepository provides batch-level persistence.
type BatchRepository struct {
	pool *pgxpool.Pool
}

// NewBatchRepository constructs a BatchRepository.
func NewBatchRepository(pool *pgxpool.Pool) *BatchRepository {
	return &BatchRepository{pool: pool}
}

// Create inserts a batch and all of its prompts in a single transaction so the
// enqueue is atomic. Prompt IDs are generated here.
func (r *BatchRepository) Create(ctx context.Context, batch *domain.Batch, prompts []string, maxAttempts int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO batches (id, status, total_count) VALUES ($1, $2, $3)`,
		batch.ID, batch.Status, batch.TotalCount,
	); err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}

	now := time.Now()
	rows := make([][]any, len(prompts))
	for i, text := range prompts {
		rows[i] = []any{
			uuid.New(),           // id
			batch.ID,             // batch_id
			i,                    // seq
			text,                 // prompt_text
			domain.PromptPending, // status
			0,                    // attempts
			maxAttempts,          // max_attempts
			now,                  // next_retry_at
		}
	}

	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{"prompts"},
		[]string{"id", "batch_id", "seq", "prompt_text", "status", "attempts", "max_attempts", "next_retry_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("copy prompts: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Counts returns terminal-state counts for a batch.
func (r *BatchRepository) Counts(ctx context.Context, batchID uuid.UUID) (domain.BatchCounts, error) {
	var c domain.BatchCounts
	err := r.pool.QueryRow(ctx, `
		SELECT
			count(*)                                        AS total,
			count(*) FILTER (WHERE status = 'succeeded')    AS succeeded,
			count(*) FILTER (WHERE status = 'failed')       AS failed
		FROM prompts
		WHERE batch_id = $1`,
		batchID,
	).Scan(&c.Total, &c.Succeeded, &c.Failed)
	if err != nil {
		return domain.BatchCounts{}, fmt.Errorf("count prompts: %w", err)
	}
	return c, nil
}

// GetSucceeded returns all successfully completed prompts for a batch, ordered
// by sequence, used to build the aggregated result.
func (r *BatchRepository) GetSucceeded(ctx context.Context, batchID uuid.UUID) ([]domain.Prompt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, batch_id, seq, prompt_text, response, attempts
		FROM prompts
		WHERE batch_id = $1 AND status = 'succeeded'
		ORDER BY seq`,
		batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("query succeeded prompts: %w", err)
	}
	defer rows.Close()

	var out []domain.Prompt
	for rows.Next() {
		var p domain.Prompt
		if err := rows.Scan(&p.ID, &p.BatchID, &p.Seq, &p.Text, &p.Response, &p.Attempts); err != nil {
			return nil, fmt.Errorf("scan prompt: %w", err)
		}
		p.Status = domain.PromptSucceeded
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompts: %w", err)
	}
	return out, nil
}

// Complete atomically marks a batch completed and stores its result, but only
// if it is not already completed. Returns true if this call performed the
// transition (guaranteeing exactly-once finalization under concurrent workers).
func (r *BatchRepository) Complete(ctx context.Context, batchID uuid.UUID, result []byte) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE batches
		SET status = 'completed', result = $2, completed_at = now()
		WHERE id = $1 AND status <> 'completed'`,
		batchID, result,
	)
	if err != nil {
		return false, fmt.Errorf("complete batch: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Get returns a batch by ID (used by health/debug paths and future status API).
func (r *BatchRepository) Get(ctx context.Context, batchID uuid.UUID) (*domain.Batch, error) {
	var b domain.Batch
	err := r.pool.QueryRow(ctx, `
		SELECT id, status, total_count, created_at, completed_at, result
		FROM batches WHERE id = $1`,
		batchID,
	).Scan(&b.ID, &b.Status, &b.TotalCount, &b.CreatedAt, &b.CompletedAt, &b.Result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	return &b, nil
}

// Ping verifies database connectivity.
func (r *BatchRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}
