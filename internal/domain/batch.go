package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// BatchStatus is the lifecycle state of a batch.
type BatchStatus string

const (
	BatchPending    BatchStatus = "pending"
	BatchProcessing BatchStatus = "processing"
	BatchCompleted  BatchStatus = "completed"
)

// Batch is a group of prompts submitted together.
type Batch struct {
	ID          uuid.UUID
	Status      BatchStatus
	TotalCount  int
	CreatedAt   time.Time
	CompletedAt *time.Time
	Result      json.RawMessage
}

// BatchCounts summarizes prompt outcomes for a batch.
type BatchCounts struct {
	Total     int
	Succeeded int
	Failed    int
}

// Remaining returns the number of prompts not yet in a terminal state.
func (c BatchCounts) Remaining() int {
	return c.Total - c.Succeeded - c.Failed
}

// Done reports whether every prompt in the batch has reached a terminal state.
func (c BatchCounts) Done() bool {
	return c.Total > 0 && c.Remaining() == 0
}
