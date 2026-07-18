package domain

import (
	"time"

	"github.com/google/uuid"
)

// PromptStatus is the lifecycle state of a single prompt in the durable queue.
type PromptStatus string

const (
	PromptPending    PromptStatus = "pending"
	PromptProcessing PromptStatus = "processing"
	PromptSucceeded  PromptStatus = "succeeded"
	PromptFailed     PromptStatus = "failed"
)

// Prompt is one unit of work within a batch. It is also the durable queue row:
// Status, Attempts, and NextRetryAt drive claiming and retry scheduling.
type Prompt struct {
	ID          uuid.UUID
	BatchID     uuid.UUID
	Seq         int
	Text        string
	Status      PromptStatus
	Attempts    int
	MaxAttempts int
	NextRetryAt time.Time
	Response    string
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
