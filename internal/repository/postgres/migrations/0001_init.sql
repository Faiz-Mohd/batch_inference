-- Schema for the batch inference service.
-- The prompts table doubles as a durable work queue.

CREATE TABLE IF NOT EXISTS batches (
    id           UUID PRIMARY KEY,
    status       TEXT NOT NULL DEFAULT 'pending',
    total_count  INTEGER NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    result       JSONB
);

CREATE TABLE IF NOT EXISTS prompts (
    id            UUID PRIMARY KEY,
    batch_id      UUID NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    seq           INTEGER NOT NULL,
    prompt_text   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 5,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    response      TEXT,
    error         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Supports the claim query: pending rows that are due, ordered by schedule.
CREATE INDEX IF NOT EXISTS idx_prompts_claim
    ON prompts (next_retry_at)
    WHERE status = 'pending';

-- Supports per-batch counting / completion checks.
CREATE INDEX IF NOT EXISTS idx_prompts_batch_id
    ON prompts (batch_id);
