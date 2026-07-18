package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

// fakeBatchRepo is a hand-written in-memory BatchRepository.
type fakeBatchRepo struct {
	createErr error
	created   *domain.Batch
	prompts   []string

	batch  *domain.Batch
	counts domain.BatchCounts

	succeeded []domain.Prompt

	completeCalls  int
	completeResult []byte
	completeErr    error
}

func (f *fakeBatchRepo) Create(_ context.Context, batch *domain.Batch, prompts []string, _ int) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = batch
	f.prompts = prompts
	return nil
}

func (f *fakeBatchRepo) Get(_ context.Context, _ uuid.UUID) (*domain.Batch, error) {
	return f.batch, nil
}

func (f *fakeBatchRepo) Counts(_ context.Context, _ uuid.UUID) (domain.BatchCounts, error) {
	return f.counts, nil
}

func (f *fakeBatchRepo) GetSucceeded(_ context.Context, _ uuid.UUID) ([]domain.Prompt, error) {
	return f.succeeded, nil
}

func (f *fakeBatchRepo) Complete(_ context.Context, _ uuid.UUID, result []byte) (bool, error) {
	if f.completeErr != nil {
		return false, f.completeErr
	}
	f.completeCalls++
	f.completeResult = result
	return true, nil
}

// requeueCall records one PromptRepository.Requeue invocation.
type requeueCall struct {
	id             uuid.UUID
	nextRetryAt    time.Time
	lastErr        string
	consumeAttempt bool
}

// fakePromptRepo is a hand-written in-memory PromptRepository.
type fakePromptRepo struct {
	claimed   []domain.Prompt
	succeeds  []uuid.UUID
	requeues  []requeueCall
	fails     []uuid.UUID
	recovered int64
}

func (f *fakePromptRepo) Claim(_ context.Context, _ int) ([]domain.Prompt, error) {
	out := f.claimed
	f.claimed = nil
	return out, nil
}

func (f *fakePromptRepo) Succeed(_ context.Context, id uuid.UUID, _ string) error {
	f.succeeds = append(f.succeeds, id)
	return nil
}

func (f *fakePromptRepo) Requeue(_ context.Context, id uuid.UUID, nextRetryAt time.Time, lastErr string, consumeAttempt bool) error {
	f.requeues = append(f.requeues, requeueCall{
		id:             id,
		nextRetryAt:    nextRetryAt,
		lastErr:        lastErr,
		consumeAttempt: consumeAttempt,
	})
	return nil
}

func (f *fakePromptRepo) Fail(_ context.Context, id uuid.UUID, _ string) error {
	f.fails = append(f.fails, id)
	return nil
}

func (f *fakePromptRepo) RecoverStuck(_ context.Context) (int64, error) {
	return f.recovered, nil
}
