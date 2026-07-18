# Batch Inference Service

A production-ready Go service that accepts a JSON array of prompts, acknowledges
immediately, and processes them concurrently through a **bounded, durable,
Postgres-backed worker pool** against a (mock) rate-limited inference endpoint.
It retries with exponential backoff (honoring `Retry-After`) so no prompt is
dropped, and aggregates all successful inferences into a final JSON document
stored in the database once the batch completes.

Built to deploy on **DigitalOcean App Platform** with **Managed Postgres**.

## Architecture

```
POST /v1/batches ──▶ validate ──▶ persist batch + prompt rows ──▶ 202 {batch_id}
                                          │
                     (Postgres is the durable queue: each prompt is a row)
                                          │
        dispatcher ── claim (FOR UPDATE SKIP LOCKED) ──▶ bounded worker pool
                                          │
             workers ── HTTP ──▶ mock rate-limited inference endpoint
                                          │
          success│retry(backoff via next_retry_at)│fail(after max attempts)
                                          │
             on last terminal prompt ──▶ aggregate succeeded ──▶ batches.result
```

Durability comes from Postgres being the queue. Every prompt is a row with
`status`, `attempts`, and `next_retry_at`. Workers claim rows atomically with
`FOR UPDATE SKIP LOCKED`, so nothing is lost on restart and retries are simply
rows scheduled for the future. This also means you can run multiple instances
safely.

### Layered structure (handler -> service -> repository)

```
cmd/server/            entrypoint, DI wiring, graceful shutdown
internal/config/       configuration (env essentials + config.yaml tuning)
internal/domain/       core types + typed retryable errors (no deps)
internal/logging/      structured slog logger + context helpers
internal/handler/      gin HTTP layer, middleware, DTOs, mock endpoint
internal/service/      business logic: batch service, processor (pool), retry, aggregator
internal/repository/   Postgres access behind interfaces + embedded migrations
internal/inference/    HTTP client to the inference endpoint
```

Dependencies point inward via interfaces: handlers depend on service interfaces,
services depend on repository/inference interfaces, and `main.go` wires the
concrete implementations.

## API

### `POST /v1/batches`

The request body must be a JSON array of prompt strings:

```json
["prompt one", "prompt two"]
```

Returns `202 Accepted` immediately after validation:

```json
{ "batch_id": "b1f2...", "accepted": 2, "status": "pending" }
```

Validation errors return `400` with `{ "error": "..." }`; bodies larger than the
configured limit return `413`.

### `GET /v1/batches/{id}`

Returns the batch status with live per-prompt counts, and the aggregated result
document once the batch completes:

```json
{
  "batch_id": "b1f2...",
  "status": "completed",
  "total": 3,
  "succeeded": 3,
  "failed": 0,
  "created_at": "2026-07-18T06:00:00Z",
  "completed_at": "2026-07-18T06:00:05Z",
  "result": { "batch_id": "b1f2...", "results": ["..."] }
}
```

Unknown IDs return `404`; malformed IDs return `400`.

### `GET /healthz`

Returns `200 {"status":"ok"}` when the database is reachable, else `503`. Used
as the App Platform health check.

### `POST /mock/infer`

The built-in mock rate-limited inference endpoint. Enforces a token-bucket rate
limit (returns `429` + `Retry-After` when exceeded) and can inject latency and
random `5xx` errors to exercise the retry path.

## Running locally

With Docker Compose (service + Postgres):

```bash
docker compose up --build
```

Then submit a batch:

```bash
curl -s -X POST localhost:8080/v1/batches \
  -H 'Content-Type: application/json' \
  -d '["hello","world","how are you"]'
```

Without Docker (needs a local Postgres):

```bash
cp .env.example .env          # edit DATABASE_URL as needed
export $(grep -v '^#' .env | xargs)
go run ./cmd/server
```

## Configuration

Configuration is split in two:

- **Environment** — only deployment-essential / secret values (see `.env.example`):

  | Variable | Default | Description |
  | --- | --- | --- |
  | `DATABASE_URL` | (required) | Postgres connection string (secret) |
  | `PORT` | `8080` | HTTP listen port (provided by the platform) |
  | `CONFIG_PATH` | `config.yaml` | Path to the YAML tuning file |

- **`config.yaml`** — all tunable behavior, loaded at startup. Any omitted key
  falls back to a built-in default, and if the file is missing entirely the
  service runs on defaults. Keys:

  | Section / key | Default | Description |
  | --- | --- | --- |
  | `worker.pool_size` | `8` | Concurrent workers (bounds load on the endpoint) |
  | `worker.claim_batch_size` | `8` | Prompts claimed per poll |
  | `worker.poll_interval` | `500ms` | Idle dispatcher poll interval |
  | `retry.max_attempts` | `5` | Attempts before a prompt is marked failed |
  | `retry.base_backoff` | `250ms` | Base for exponential backoff |
  | `retry.max_backoff` | `30s` | Backoff ceiling |
  | `validation.max_batch_size` | `10000` | Max prompts per request |
  | `validation.max_prompt_len` | `8192` | Max characters per prompt |
  | `inference.url` | `.../mock/infer` | Endpoint the workers call |
  | `inference.request_timeout` | `10s` | Per-request timeout |
  | `mock.rate_per_sec` | `20` | Mock endpoint token refill rate |
  | `mock.burst` | `10` | Mock endpoint bucket size |
  | `mock.fail_rate` | `0.05` | Probability of an injected 5xx |
  | `mock.max_latency` | `100ms` | Max simulated processing latency |
  | `logging.level` | `info` | `debug`/`info`/`warn`/`error` |
  | `logging.format` | `json` | `json` (prod) or `text` (dev) |
  | `lifecycle.shutdown_timeout` | `20s` | Graceful shutdown budget |

  In the Docker image `config.yaml` is baked in at `/config.yaml`; mount your own
  and/or set `CONFIG_PATH` to override.

## Logging

Structured JSON logs via `slog`. Every HTTP request gets a `request_id`
(honoring an inbound `X-Request-ID`), and that logger is propagated through the
context so worker/service logs carry `batch_id`, `prompt_id`, `attempt`, and
`worker_id` for easy debugging. Set `logging.format: text` in `config.yaml` for
readable local logs.

## Deployment (DigitalOcean App Platform)

The spec in `.do/app.yaml` builds from the `Dockerfile`, provisions Managed
Postgres, binds `DATABASE_URL`, and health-checks `/healthz`.

```bash
doctl apps create --spec .do/app.yaml
```

Update the `github.repo`/`branch` in the spec to point at your repository.

## Notes / future work

- If aggregated results grow very large, swap the `batches.result` jsonb write
  for an object-storage (Spaces) pointer -- the aggregator is isolated for this.
```
