package handler

import "github.com/google/uuid"

// createBatchRequest is the accepted request body: a JSON array of prompt
// strings, e.g. ["prompt one", "prompt two"].
type createBatchRequest []string

// Prompts returns the request as a plain string slice.
func (r createBatchRequest) Prompts() []string { return r }

// createBatchResponse is the immediate ack returned after validation.
type createBatchResponse struct {
	BatchID  uuid.UUID `json:"batch_id"`
	Accepted int       `json:"accepted"`
	Status   string    `json:"status"`
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
