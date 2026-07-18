package service

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

// BatchResult is the aggregated final JSON document persisted on completion.
type BatchResult struct {
	BatchID     uuid.UUID      `json:"batch_id"`
	Total       int            `json:"total"`
	Succeeded   int            `json:"succeeded"`
	Failed      int            `json:"failed"`
	GeneratedAt time.Time      `json:"generated_at"`
	Results     []PromptResult `json:"results"`
}

// PromptResult is a single successful inference within the aggregated output.
type PromptResult struct {
	PromptID uuid.UUID `json:"prompt_id"`
	Seq      int       `json:"seq"`
	Prompt   string    `json:"prompt"`
	Response string    `json:"response"`
}

// buildResult assembles the final JSON document from the succeeded prompts and
// the batch counts.
func buildResult(batchID uuid.UUID, counts domain.BatchCounts, succeeded []domain.Prompt) ([]byte, error) {
	results := make([]PromptResult, 0, len(succeeded))
	for _, p := range succeeded {
		results = append(results, PromptResult{
			PromptID: p.ID,
			Seq:      p.Seq,
			Prompt:   p.Text,
			Response: p.Response,
		})
	}

	doc := BatchResult{
		BatchID:     batchID,
		Total:       counts.Total,
		Succeeded:   counts.Succeeded,
		Failed:      counts.Failed,
		GeneratedAt: time.Now().UTC(),
		Results:     results,
	}
	return json.Marshal(doc)
}
