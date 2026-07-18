package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
	"github.com/example/batch-inference/internal/logging"
)

// ProcessorConfig tunes the worker pool and retry policy.
type ProcessorConfig struct {
	PoolSize     int
	ClaimBatch   int
	PollInterval time.Duration
	BaseBackoff  time.Duration
	MaxBackoff   time.Duration
}

// Processor runs the durable-queue dispatcher and a bounded pool of workers that
// call the inference endpoint, apply retry/backoff, and finalize batches.
type Processor struct {
	prompts PromptRepository
	batches BatchRepository
	client  InferenceClient
	cfg     ProcessorConfig
	logger  *slog.Logger
}

// NewProcessor constructs a Processor.
func NewProcessor(prompts PromptRepository, batches BatchRepository, client InferenceClient, cfg ProcessorConfig, logger *slog.Logger) *Processor {
	return &Processor{
		prompts: prompts,
		batches: batches,
		client:  client,
		cfg:     cfg,
		logger:  logger,
	}
}

// Run starts the dispatcher and workers, blocking until ctx is cancelled and all
// in-flight work has drained. Intended to be called in its own goroutine.
func (p *Processor) Run(ctx context.Context) {
	baseCtx := logging.WithContext(ctx, p.logger)

	jobs := make(chan domain.Prompt)
	var wg sync.WaitGroup

	for i := 0; i < p.cfg.PoolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			p.worker(baseCtx, workerID, jobs)
		}(i)
	}

	p.logger.Info("processor started", "workers", p.cfg.PoolSize, "claim_batch", p.cfg.ClaimBatch)

	p.dispatch(baseCtx, jobs)

	// Dispatcher returned (ctx cancelled). Close the channel and wait for workers
	// to finish their current jobs, then return.
	close(jobs)
	wg.Wait()
	p.logger.Info("processor stopped")
}

// dispatch polls the queue and feeds claimed prompts to workers. Sending on the
// unbuffered jobs channel provides natural backpressure: we never hold more than
// pool-size + in-flight claimed rows.
func (p *Processor) dispatch(ctx context.Context, jobs chan<- domain.Prompt) {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		claimed, err := p.prompts.Claim(ctx, p.cfg.ClaimBatch)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.logger.Error("claim failed", "err", err)
			if !sleep(ctx, p.cfg.PollInterval) {
				return
			}
			continue
		}

		if len(claimed) == 0 {
			// No work; wait for the next tick.
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}

		p.logger.Debug("claimed prompts", "count", len(claimed))
		for _, prompt := range claimed {
			select {
			case <-ctx.Done():
				return
			case jobs <- prompt:
			}
		}
	}
}

// worker processes prompts until the jobs channel is closed.
func (p *Processor) worker(ctx context.Context, workerID int, jobs <-chan domain.Prompt) {
	for prompt := range jobs {
		p.process(ctx, workerID, prompt)
	}
}

// process runs one inference attempt and records the outcome.
func (p *Processor) process(ctx context.Context, workerID int, prompt domain.Prompt) {
	logger := logging.FromContext(ctx).With(
		"worker_id", workerID,
		"batch_id", prompt.BatchID,
		"prompt_id", prompt.ID,
		"attempt", prompt.Attempts,
	)

	response, err := p.client.Infer(ctx, prompt.Text)
	if err != nil {
		p.handleFailure(ctx, logger, prompt, err)
		return
	}

	if err := p.prompts.Succeed(ctx, prompt.ID, response); err != nil {
		logger.Error("failed to persist success", "err", err)
		return
	}
	logger.Info("prompt succeeded")
	p.finalizeIfDone(ctx, logger, prompt.BatchID)
}

// handleFailure decides whether to requeue with backoff or fail permanently.
func (p *Processor) handleFailure(ctx context.Context, logger *slog.Logger, prompt domain.Prompt, err error) {
	re, retryable := domain.AsRetryable(err)

	// Rate limiting (429) is not the prompt's fault, so it never consumes the
	// retry budget; genuine transient errors (5xx / transport) retry until the
	// budget is spent.
	rateLimited := retryable && re.RateLimited
	if rateLimited || (retryable && prompt.Attempts < prompt.MaxAttempts) {
		delay := backoff(prompt.Attempts, p.cfg.BaseBackoff, p.cfg.MaxBackoff, re.RetryAfter)
		next := time.Now().Add(delay)
		if rerr := p.prompts.Requeue(ctx, prompt.ID, next, err.Error(), !rateLimited); rerr != nil {
			logger.Error("failed to requeue prompt", "err", rerr)
			return
		}
		logger.Warn("prompt requeued for retry",
			"err", err,
			"rate_limited", rateLimited,
			"delay_ms", delay.Milliseconds(),
			"next_retry_at", next.UTC(),
			"max_attempts", prompt.MaxAttempts,
		)
		return
	}

	if ferr := p.prompts.Fail(ctx, prompt.ID, err.Error()); ferr != nil {
		logger.Error("failed to mark prompt failed", "err", ferr)
		return
	}
	logger.Error("prompt failed permanently", "err", err, "attempts", prompt.Attempts)
	p.finalizeIfDone(ctx, logger, prompt.BatchID)
}

// finalizeIfDone checks whether every prompt in the batch is terminal and, if so,
// aggregates the successful results into the final JSON and marks the batch
// completed. The repository's guarded update makes this exactly-once.
func (p *Processor) finalizeIfDone(ctx context.Context, logger *slog.Logger, batchID uuid.UUID) {
	counts, err := p.batches.Counts(ctx, batchID)
	if err != nil {
		logger.Error("failed to count batch prompts", "err", err)
		return
	}
	if !counts.Done() {
		return
	}

	succeeded, err := p.batches.GetSucceeded(ctx, batchID)
	if err != nil {
		logger.Error("failed to fetch succeeded prompts", "err", err)
		return
	}

	result, err := buildResult(batchID, counts, succeeded)
	if err != nil {
		logger.Error("failed to build batch result", "err", err)
		return
	}

	completed, err := p.batches.Complete(ctx, batchID, result)
	if err != nil {
		logger.Error("failed to complete batch", "err", err)
		return
	}
	if completed {
		logger.Info("batch completed",
			"total", counts.Total,
			"succeeded", counts.Succeeded,
			"failed", counts.Failed,
		)
	}
}

// sleep waits for d or until ctx is cancelled. Returns false if cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
