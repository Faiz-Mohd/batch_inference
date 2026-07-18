package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

func TestCreateBatchValidation(t *testing.T) {
	svc := NewBatchService(&fakeBatchRepo{}, 3, 10, 5)

	tests := []struct {
		name    string
		prompts []string
		wantErr error
	}{
		{"empty batch", nil, domain.ErrEmptyBatch},
		{"too many prompts", []string{"a", "b", "c", "d"}, domain.ErrBatchTooLarge},
		{"empty prompt", []string{"a", "   "}, domain.ErrEmptyPrompt},
		{"prompt too long", []string{strings.Repeat("x", 11)}, domain.ErrPromptTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateBatch(context.Background(), tt.prompts)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got err %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateBatchTrimsAndPersists(t *testing.T) {
	repo := &fakeBatchRepo{}
	svc := NewBatchService(repo, 10, 100, 5)

	batch, err := svc.CreateBatch(context.Background(), []string{"  hello  ", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batch.Status != domain.BatchPending {
		t.Errorf("status = %q, want %q", batch.Status, domain.BatchPending)
	}
	if batch.TotalCount != 2 {
		t.Errorf("total = %d, want 2", batch.TotalCount)
	}
	if repo.prompts[0] != "hello" || repo.prompts[1] != "world" {
		t.Errorf("persisted prompts = %q, want trimmed values", repo.prompts)
	}
}

func TestCreateBatchRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	svc := NewBatchService(&fakeBatchRepo{createErr: repoErr}, 10, 100, 5)

	if _, err := svc.CreateBatch(context.Background(), []string{"hello"}); !errors.Is(err, repoErr) {
		t.Fatalf("got err %v, want %v", err, repoErr)
	}
}

func TestGetBatch(t *testing.T) {
	id := uuid.New()
	repo := &fakeBatchRepo{
		batch:  &domain.Batch{ID: id, Status: domain.BatchPending, TotalCount: 3},
		counts: domain.BatchCounts{Total: 3, Succeeded: 1, Failed: 1},
	}
	svc := NewBatchService(repo, 10, 100, 5)

	batch, counts, err := svc.GetBatch(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batch.ID != id {
		t.Errorf("batch ID = %v, want %v", batch.ID, id)
	}
	if counts.Succeeded != 1 || counts.Failed != 1 {
		t.Errorf("counts = %+v, want succeeded=1 failed=1", counts)
	}
}

func TestGetBatchNotFound(t *testing.T) {
	svc := NewBatchService(&fakeBatchRepo{}, 10, 100, 5)

	batch, _, err := svc.GetBatch(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if batch != nil {
		t.Fatalf("batch = %+v, want nil for unknown ID", batch)
	}
}
