package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/example/batch-inference/internal/domain"
)

// fakeBatchService is a hand-written BatchService test double.
type fakeBatchService struct {
	createErr error
	batch     *domain.Batch
	counts    domain.BatchCounts
}

func (f *fakeBatchService) CreateBatch(_ context.Context, prompts []string) (*domain.Batch, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &domain.Batch{ID: uuid.New(), Status: domain.BatchPending, TotalCount: len(prompts)}, nil
}

func (f *fakeBatchService) GetBatch(_ context.Context, _ uuid.UUID) (*domain.Batch, domain.BatchCounts, error) {
	return f.batch, f.counts, nil
}

func newTestEngine(svc BatchService, maxBodyBytes int64) *gin.Engine {
	router := Router{
		Batch:  NewBatchHandler(svc, maxBodyBytes),
		Health: NewHealthHandler(pingOK{}),
		Logger: slog.New(slog.DiscardHandler),
	}
	return router.Build()
}

type pingOK struct{}

func (pingOK) Ping(context.Context) error { return nil }

func TestCreateBatchAccepted(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{}, 1<<20)

	req := httptest.NewRequest(http.MethodPost, "/v1/batches", strings.NewReader(`["hello","world"]`))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body)
	}
	var resp struct {
		BatchID  uuid.UUID `json:"batch_id"`
		Accepted int       `json:"accepted"`
		Status   string    `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Accepted != 2 || resp.Status != "pending" || resp.BatchID == uuid.Nil {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestCreateBatchMalformedBody(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{}, 1<<20)

	req := httptest.NewRequest(http.MethodPost, "/v1/batches", strings.NewReader(`{"not":"an array"}`))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateBatchValidationError(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{createErr: domain.ErrBatchTooLarge}, 1<<20)

	req := httptest.NewRequest(http.MethodPost, "/v1/batches", strings.NewReader(`["hello"]`))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateBatchBodyTooLarge(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{}, 16)

	req := httptest.NewRequest(http.MethodPost, "/v1/batches", strings.NewReader(`["`+strings.Repeat("x", 100)+`"]`))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestGetBatchFound(t *testing.T) {
	id := uuid.New()
	engine := newTestEngine(&fakeBatchService{
		batch:  &domain.Batch{ID: id, Status: domain.BatchCompleted, TotalCount: 2, Result: json.RawMessage(`{"ok":true}`)},
		counts: domain.BatchCounts{Total: 2, Succeeded: 2},
	}, 1<<20)

	req := httptest.NewRequest(http.MethodGet, "/v1/batches/"+id.String(), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body)
	}
	var resp struct {
		BatchID   uuid.UUID       `json:"batch_id"`
		Status    string          `json:"status"`
		Succeeded int             `json:"succeeded"`
		Result    json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.BatchID != id || resp.Status != "completed" || resp.Succeeded != 2 || len(resp.Result) == 0 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestGetBatchNotFound(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{}, 1<<20)

	req := httptest.NewRequest(http.MethodGet, "/v1/batches/"+uuid.NewString(), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGetBatchInvalidID(t *testing.T) {
	engine := newTestEngine(&fakeBatchService{}, 1<<20)

	req := httptest.NewRequest(http.MethodGet, "/v1/batches/not-a-uuid", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
