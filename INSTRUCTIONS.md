# Real-Time CPI Webcrawler – Detailed Instructions

This document expands on the README to describe configuration, APIs, operations, and troubleshooting for the crawler service.

---

## 1. Architecture Recap

1. **API Layer (chi):** Validates requests, writes `Job` entries, enqueues work, and exposes status/results plus health/metrics endpoints. Authentication is optional via API key middleware.
2. **Dispatcher & Queue:** In-memory FIFO queue with bounded capacity feeds a worker pool sized by `crawler.concurrency`.
3. **Workers:** For each job URL:
   - Colly probe fetch (robots-aware, configurable delay/UA).
   - Run heuristics; if eligible, promote to chromedp headless fetch with max parallel controls.
   - Hash HTML, persist to BlobStore, save `PageRecord`, publish message to Pub/Sub interface.
   - Update counters and final job status.
4. **Storage:** Currently in-memory stores; swap for DB/Blob services for persistence beyond process lifetime.
5. **Observability:** Zap logging for structured debug/info/error plus Prometheus metrics (HTTP, queue, jobs, pages); placement hooks exist for OTEL spans.

---

## 2. Configuration Guide

Configuration is merged from `config.yaml` and environment variables (prefix `CRAWLER_`, `.` replaced with `_`). All fields are documented in `internal/config/config.go`. Highlights:

### 2.1 Server

```yaml
server:
  port: 8080
```

### 2.2 Authentication

```yaml
auth:
  enabled: true
  api_key: "<shared-secret>"
```

Requests must include `X-API-Key` header or `?api_key=` query parameter when enabled.

### 2.3 Crawler

```yaml
crawler:
  concurrency: 4
  user_agent: "real-cpi-bot/0.1"
  ignore_robots: false
  max_depth_default: 1
  max_pages_default: 10
  queue_depth: 64
```

- `concurrency` controls how many workers are spawned.
- `ignore_robots` flips the default for custom job requests (clients can override).
- `max_depth_default` / `max_pages_default` seed job parameters when omitted.
- `queue_depth` bounds the in-memory queue; use it to apply backpressure.
- `user_agent` is propagated to both Colly and headless fetchers.

### 2.4 HTTP / Retry

```yaml
http:
  timeout_seconds: 15
```

- `timeout_seconds` doubles as the default crawl budget per job and the Colly request timeout.

### 2.5 Headless

```yaml
headless:
  enabled: true
  max_parallel: 2
  nav_timeout_seconds: 30
  promotion_threshold: 2048
```

- `promotion_threshold` is the minimum HTML byte length the detector expects before considering a page “complete”; smaller documents with high script density are promoted to headless.

### 2.6 Storage & Pub/Sub

```yaml
storage:
  backend: "local"
  bucket: ""
  prefix: "crawl"
  content_type: "text/html; charset=utf-8"
  local:
    base_dir: "tmp/storage"

database:
  dsn: ""
  retrieval_table: "crawl"
  progress_table: "job_runs"
  stats_table: "job_stats"
  max_conns: 4
  min_conns: 0
  max_conn_lifetime: 0s

pubsub:
  project_id: ""
  topic_name: ""
```

- `storage.backend` picks the implementation (`memory`, `gcs`, or `local`).
- `storage.bucket` names the GCS bucket when `storage.backend = gcs`.
- `storage.local.base_dir` specifies the directory to store files when `storage.backend = local`.
- `storage.prefix` scopes blob paths (`<prefix>/crawl/...`); leave blank to write at the root.
- `storage.content_type` is stored alongside each blob for downstream consumers.
- `database.*` controls the Postgres pool used to persist retrieval rows (set `dsn` to enable).
- `database.retrieval_table` names the table for storing individual page retrievals (default: `crawl`).
- `database.progress_table` names the table for tracking job run lifecycle (default: `job_runs`).
- `database.stats_table` names the table for aggregated site statistics (default: `job_stats`).
- `pubsub.project_id` + `pubsub.topic_name` enable Cloud Pub/Sub publishing; leave blank to disable.

### 2.7 Logging

```yaml
logging:
  development: true       # colorized dev logs; false → JSON prod logs
```

### 2.8 Metrics & Progress

```yaml
metrics:
  enabled: true
  path: "/metrics"

progress:
  enabled: true
  buffer_size: 4096
  sink_timeout_ms: 2000
  log_enabled: false
  batch:
    max_events: 1000
    max_wait_ms: 500
```

- `metrics.enabled` toggles Prometheus metrics endpoint exposition.
- `metrics.path` sets the HTTP path for the metrics endpoint (default: `/metrics`).
- `progress.enabled` toggles the progress tracking subsystem.
- `progress.buffer_size` configures hub channel capacity for buffering progress events.
- `progress.batch.max_events` flushes after this many events accumulate.
- `progress.batch.max_wait_ms` flushes after this many milliseconds even if under max_events.
- `progress.sink_timeout_ms` bounds per-sink consume calls.
- `progress.log_enabled` toggles the LogSink for debugging progress events.

### 2.9 Standard Jobs Templates

```yaml
standard_jobs:
  price-refresh:
    urls: ["https://example.com"]
    max_depth: 1
    headless_allowed: true
    respect_robots: true
```

---

### 2.10 Retrieval Persistence

The worker mirrors each successful fetch into Postgres using three tables. When `database.dsn` is provided, ensure the following tables exist:

#### Retrievals Table (default: `crawl`, configured via `database.retrieval_table`)

| Column | Type | Notes |
|--------|------|-------|
| `id uuid` | PK | Matches the blob folder `id=<uuid>` |
| `job_id uuid` | NOT NULL | Crawl/job identifier |
| `job_started_at timestamptz` | NOT NULL | When the job began |
| `partition_ts timestamptz` | NOT NULL | `retrieval_timestamp` truncated to the hour (helps time-based partitions) |
| `retrieval_timestamp timestamptz` | NOT NULL DEFAULT now() | Exact ingest time |
| `retrieval_url text` | NOT NULL | Final URL |
| `retrieval_hashcode text` | NULL | SHA256/hasher output |
| `retrieval_blob_location text` | NULL | Blob URI of `raw.html` |
| `retrieval_headers jsonb` | NULL | HTTP headers captured during fetch |
| `retrieval_status_code integer` | NULL | HTTP response status |
| `retrieval_content_type text` | NULL | MIME type used when writing `raw.html` |
| `parent_id uuid` / `parent_ts timestamptz` | NULL | Optional ancestry for promoted sub-pages |
| `created_at timestamptz` / `updated_at timestamptz` | NOT NULL DEFAULT now() | Basic audit columns |

#### Job Runs Table (default: `job_runs`, configured via `database.progress_table`)

| Column | Type | Notes |
|--------|------|-------|
| `id uuid` | PK | Run identifier (same as job_id) |
| `job_id uuid` | NOT NULL | Job identifier |
| `started_at timestamptz` | NOT NULL | When the job started |
| `finished_at timestamptz` | NULL | When the job completed |
| `status text` | NOT NULL | Job status (running, success, error) |
| `error_message text` | NULL | Error details if status is error |

#### Site Stats Table (default: `job_stats`, configured via `database.stats_table`)

| Column | Type | Notes |
|--------|------|-------|
| `job_id uuid` | NOT NULL | Associated job |
| `site text` | NOT NULL | Domain/site being crawled |
| `last_update timestamptz` | NOT NULL | Last time stats were updated |
| `visits int64` | NOT NULL DEFAULT 0 | Total page visits |
| `bytes_total int64` | NOT NULL DEFAULT 0 | Total bytes fetched |
| `fetch_2xx int64` | NOT NULL DEFAULT 0 | Count of 2xx responses |
| `fetch_3xx int64` | NOT NULL DEFAULT 0 | Count of 3xx responses |
| `fetch_4xx int64` | NOT NULL DEFAULT 0 | Count of 4xx responses |
| `fetch_5xx int64` | NOT NULL DEFAULT 0 | Count of 5xx responses |

The crawler only inserts rows; downstream services can extend these tables with additional columns for processing flags or AI pipeline state.

When Cloud Pub/Sub is enabled, each retrieval also results in a JSON payload (containing `crawl_id`, `site`, `html_blob`, `meta_blob`) being published to `pubsub.topic_name` for the AI processing tier.

## 3. API Details

### 3.1 Submit Custom Job

`POST /v1/jobs/custom`

```jsonc
{
  "urls": ["https://store.example"],
  "max_depth": 1,
  "max_pages": 20,
  "budget_seconds": 60,
  "headless_allowed": true,
  "respect_robots": true,
  "per_domain_caps": {
    "store.example": 2
  },
  "tags": {
    "campaign": "holiday"
  },
  "allow_domains": ["store.example"],
  "deny_domains": ["ads.example"]
}
```

Response: `{"job_id": "<uuid>"}` (202 Accepted or 429 if queue full).

### 3.2 Submit Standard Job

`POST /v1/jobs/standard`

```json
{
  "name": "price-refresh"
}
```

### 3.3 Get Status

`GET /v1/jobs/{job_id}/status` → `{ "job": { ... } }`.

### 3.4 Get Result

`GET /v1/jobs/{job_id}/result` → returns `Job` plus list of `PageRecord`s.

### 3.5 Cancel Job

`POST /v1/jobs/{job_id}/cancel` (best-effort; sets status to canceled, workers honor context cancellation).

### 3.6 Health/Ready/Metrics

- `/healthz` – simple 200 response.
- `/readyz` – always ready in memory-only mode; extend to check dependencies.
- `/metrics` – Prometheus registry export (HTTP latency, queue depth, job/page counters).

### 3.7 Progress Endpoints

`GET /api/jobs?status=&limit=&offset=`

Returns a list of job runs with optional filtering by status (running, success, error). Supports pagination via limit (default 50, max 500) and offset parameters.

Response: `{"jobs": [...]}`

`GET /api/jobs/{job_id}`

Returns details of a specific job run including start/finish times, status, and error messages.

Response: `{"job": {...}}`

`GET /api/jobs/{job_id}/sites?limit=&offset=`

Returns aggregated statistics per site for a given job, including visit counts, bytes transferred, and HTTP status code distributions.

Response: `{"sites": [...]}`

---

## 4. Logging & Observability

- Zap log levels:
  - `DEBUG`: queue dequeue, successful probe fetch, per-URL processing.
  - `INFO`: job submissions, headless promotions, publish events, lifecycle events.
  - `WARN`: headless promotion failures, policy blocks.
  - `ERROR`: fetch failures, persistence issues, HTTP handler panics.
- Development mode (default) uses human-readable console encoder; production writes JSON.
- Extend with Prometheus/OTEL by instrumenting `worker.persistAndPublish`, queue sizes, headless promotion counters, etc.

---

## 5. Running Locally

1. **Dependencies:** Go 1.25+, chromedp-compatible headless environment (Chromium/Chrome). Ensure `CHROME_BIN` or system Chrome is installed.
2. **Pre-commit hooks:** `pre-commit install`. Hooks include `gofumpt`, `goimports`, `go test -short -race`, `govulncheck` (with panic workaround).
3. **Start the service:**

```bash
go run ./cmd/webcrawler --config config.yaml
```

4. **Submit a job:**

```bash
curl -XPOST localhost:8080/v1/jobs/custom \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://example.com"],"headless_allowed":true}'
```

5. **Check status/results:** `curl localhost:8080/v1/jobs/<job_id>/status`.

---

## 6. Production Considerations

- **Persistence:** Replace in-memory stores with implementations backed by PostgreSQL (JobStore) and GCS/S3 (BlobStore). Provide connection pooling and migrations.
- **Queue:** Introduce priority-aware queue or integrate with systems like Pub/Sub/Redis to avoid in-memory limits.
- **Policies:** Implement robots.txt parsing, domain budgets, retry/backoff strategies, and rate limiting.
- **Authentication:** Enforce API key or OAuth; add rate limiting per client.
- **Headless Pool:** Run chromedp in isolated containers/pods; manage Chrome lifecycle and security (sandboxing).
- **Secrets:** Use Secret Manager/Workload Identity instead of config files for API keys and DSNs.
- **Monitoring:** Emit Prometheus metrics and OTEL traces; set up dashboards for job throughput, headless promotions, queue depth, error rates.
- **Deployment:** Build static binary (`CGO_ENABLED=0 go build`), package into distroless container, mount config via ConfigMap/Secrets, run with limited permissions.

---

## 7. Troubleshooting

| Symptom | Possible Cause | Mitigation |
|---------|----------------|------------|
| `govulncheck` hook panics | Upstream bug scanning `go-json-experiment/json` | Scripted hook ignores the specific panic; rerun when upstream is fixed |
| Headless fetch fails | No Chrome binary or chromedp permissions | Install Chrome/Chromium, ensure sandbox flags, check `headless.enabled` |
| Queue fills / 429 responses | `crawler.queue_depth` too low or workers hung | Increase queue size, inspect worker logs, add metrics |
| No results returned | Job pages failed or headless not allowed | Check worker logs; ensure `headless_allowed` and domain policy permit; review `respect_robots` |
| Logger sync error on shutdown | Running on STDOUT sink without flush support | Safe to ignore if sink doesn’t support Sync; message logged to stderr |

---

## 8. Extending the System

1. **Real Storage/PubSub:** Implement interfaces in `internal/storage`, `internal/publisher` with real clients.
2. **Metrics:** Prometheus registry + instrumentation in API/queue/workers for HTTP latency, queue depth, job lifecycle, headless promotions.
3. **Queues/Policies:** Introduce domain-level semaphores, circuit breakers for HTTP 429/5xx, and integrate robots parser.
4. **Integration tests:** Use GCS emulator, Pub/Sub emulator, and local HTTP fixtures for end-to-end testing.
5. **CLI/SDK:** Provide client library or CLI to submit jobs, monitor progress, and fetch results.

---

## 9. Reference

- `cmd/webcrawler/main.go` – service entrypoint.
- `internal/api` – HTTP handlers and middleware.
- `internal/worker` – crawl execution pipeline.
- `internal/config` – Viper configuration loader.
- `internal/logging` – zap logger helper.
- `internal/fetcher` – Colly and Chromedp fetchers.
- `internal/headless/detector` – promotion heuristics.
- `internal/storage/memory` / `internal/publisher/memory` – stub backends.

Feel free to tailor these components to your infrastructure—swap interfaces, extend policies, or add new outputs without touching the core API surface.
