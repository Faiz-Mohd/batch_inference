package handler

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// createBatchRequest is the accepted request body: a JSON array of prompt
// strings, e.g. ["prompt one", "prompt two"].
type createBatchRequest []string

// createBatchResponse is the immediate ack returned after validation.
type createBatchResponse struct {
	BatchID  uuid.UUID `json:"batch_id"`
	Accepted int       `json:"accepted"`
	Status   string    `json:"status"`
}

// batchStatusResponse reports a batch's progress and, once completed, its
// aggregated result.
type batchStatusResponse struct {
	BatchID     uuid.UUID       `json:"batch_id"`
	Status      string          `json:"status"`
	Total       int             `json:"total"`
	Succeeded   int             `json:"succeeded"`
	Failed      int             `json:"failed"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// errorResponse is the standard error envelope.
type errorResponse struct {
	Error string `json:"error"`
}

// inferRequest / inferResponse are the mock inference endpoint payloads.
type inferRequest struct {
	Prompt string `json:"prompt"`
}

type inferResponse struct {
	Completion string `json:"completion"`
}
