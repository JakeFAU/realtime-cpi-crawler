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
  prefix: "pages"
  content_type: "text/html; charset=utf-8"

pubsub:
  topic_name: ""
```

- `storage.prefix` scopes blob paths (`<prefix>/<job>/<hash>.html`); leave blank to write at the root.
- `storage.content_type` is stored alongside each blob for downstream consumers.
- `pubsub.topic_name` enables publish-on-completion; leave empty to disable publishing.

### 2.7 Logging

```yaml
logging:
  development: true       # colorized dev logs; false → JSON prod logs
```

### 2.8 Standard Jobs Templates

```yaml
standard_jobs:
  price-refresh:
    urls: ["https://example.com"]
    max_depth: 1
    headless_allowed: true
    respect_robots: true
```

---

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
