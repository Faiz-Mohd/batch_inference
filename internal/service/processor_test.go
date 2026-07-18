package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

func newTestProcessor(prompts *fakePromptRepo, batches *fakeBatchRepo) *Processor {
	return NewProcessor(prompts, batches, nil, ProcessorConfig{
		BaseBackoff: 10 * time.Millisecond,
		MaxBackoff:  100 * time.Millisecond,
	}, slog.New(slog.DiscardHandler))
}

func testPrompt(attempts, maxAttempts int) domain.Prompt {
	return domain.Prompt{
		ID:          uuid.New(),
		BatchID:     uuid.New(),
		Text:        "hello",
		Attempts:    attempts,
		MaxAttempts: maxAttempts,
	}
}

func TestHandleFailureRateLimitedDoesNotConsumeBudget(t *testing.T) {
	prompts := &fakePromptRepo{}
	p := newTestProcessor(prompts, &fakeBatchRepo{})

	// Even at the attempt budget, a 429 must requeue, not fail.
	prompt := testPrompt(5, 5)
	err := domain.NewRateLimited(errors.New("throttled"), 0)
	p.handleFailure(context.Background(), slog.New(slog.DiscardHandler), prompt, err)

	if len(prompts.fails) != 0 {
		t.Fatalf("prompt failed permanently; rate limiting must never drop prompts")
	}
	if len(prompts.requeues) != 1 {
		t.Fatalf("requeues = %d, want 1", len(prompts.requeues))
	}
	if prompts.requeues[0].consumeAttempt {
		t.Errorf("rate-limited requeue consumed the attempt budget")
	}
}

func TestHandleFailureRetryableConsumesBudget(t *testing.T) {
	prompts := &fakePromptRepo{}
	p := newTestProcessor(prompts, &fakeBatchRepo{})

	prompt := testPrompt(2, 5)
	err := domain.NewRetryable(errors.New("boom 503"), 0)
	p.handleFailure(context.Background(), slog.New(slog.DiscardHandler), prompt, err)

	if len(prompts.requeues) != 1 {
		t.Fatalf("requeues = %d, want 1", len(prompts.requeues))
	}
	if !prompts.requeues[0].consumeAttempt {
		t.Errorf("transient-error requeue must consume the attempt budget")
	}
	if !prompts.requeues[0].nextRetryAt.After(time.Now().Add(-time.Second)) {
		t.Errorf("nextRetryAt not scheduled: %v", prompts.requeues[0].nextRetryAt)
	}
}

func TestHandleFailureRetryableBudgetExhausted(t *testing.T) {
	prompts := &fakePromptRepo{}
	p := newTestProcessor(prompts, &fakeBatchRepo{})

	prompt := testPrompt(5, 5)
	err := domain.NewRetryable(errors.New("boom 503"), 0)
	p.handleFailure(context.Background(), slog.New(slog.DiscardHandler), prompt, err)

	if len(prompts.requeues) != 0 {
		t.Fatalf("requeued a prompt whose budget is exhausted")
	}
	if len(prompts.fails) != 1 {
		t.Fatalf("fails = %d, want 1", len(prompts.fails))
	}
}

func TestHandleFailureNonRetryableFailsImmediately(t *testing.T) {
	prompts := &fakePromptRepo{}
	p := newTestProcessor(prompts, &fakeBatchRepo{})

	prompt := testPrompt(1, 5)
	p.handleFailure(context.Background(), slog.New(slog.DiscardHandler), prompt, errors.New("bad request"))

	if len(prompts.requeues) != 0 {
		t.Fatalf("requeued a non-retryable failure")
	}
	if len(prompts.fails) != 1 {
		t.Fatalf("fails = %d, want 1", len(prompts.fails))
	}
}

func TestFinalizeIfDoneSkipsIncompleteBatch(t *testing.T) {
	batches := &fakeBatchRepo{counts: domain.BatchCounts{Total: 3, Succeeded: 1, Failed: 1}}
	p := newTestProcessor(&fakePromptRepo{}, batches)

	p.finalizeIfDone(context.Background(), slog.New(slog.DiscardHandler), uuid.New())

	if batches.completeCalls != 0 {
		t.Fatalf("completed a batch with %d prompts still in flight", batches.counts.Remaining())
	}
}

func TestFinalizeIfDoneCompletesAndAggregates(t *testing.T) {
	batchID := uuid.New()
	batches := &fakeBatchRepo{
		counts: domain.BatchCounts{Total: 2, Succeeded: 1, Failed: 1},
		succeeded: []domain.Prompt{
			{ID: uuid.New(), BatchID: batchID, Seq: 0, Text: "hello", Response: "world"},
		},
	}
	p := newTestProcessor(&fakePromptRepo{}, batches)

	p.finalizeIfDone(context.Background(), slog.New(slog.DiscardHandler), batchID)

	if batches.completeCalls != 1 {
		t.Fatalf("completeCalls = %d, want 1", batches.completeCalls)
	}

	var result BatchResult
	if err := json.Unmarshal(batches.completeResult, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if result.BatchID != batchID || result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Errorf("unexpected aggregate: %+v", result)
	}
	if len(result.Results) != 1 || result.Results[0].Response != "world" {
		t.Errorf("unexpected results: %+v", result.Results)
	}
}
