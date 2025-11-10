# Real-Time CPI Webcrawler

This service accepts crawl jobs over HTTP, executes them asynchronously with Colly plus chromedp for headless promotion, stores artifacts in a blob store, persists metadata, and publishes completion notifications for downstream AI pipelines.

---

## Features

- **HTTP API (chi):** submit custom or templated jobs, poll status/results, cancel outstanding work, and expose health/ready/metrics endpoints.
- **Dispatcher & Worker Pool:** bounded concurrency with per-job admission control and per-domain policy hooks.
- **Fetcher Pipeline:** fast Colly probe with optional chromedp headless promotion based on heuristics; robots adherence and retries configurable via Viper.
- **Persistence:** raw HTML stored via blob interface; job/page metadata captured in JobStore for status/result queries.
- **Pub/Sub Notification:** publishes job updates (UUID, URL, blob URI, hash, status) for downstream consumers.
- **Observability:** structured logging via zap, middleware request logs, and exported Prometheus metrics for jobs/pages/HTTP.
- **Configuration:** `config.yaml` + environment variables loaded/validated with Viper; logging toggles for dev/prod.

---

## Quick Start

```bash
# install tools
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/mvdan/gofumpt@latest

# run unit tests (short + race)
go test ./... -short -race

# start the crawler (defaults to :8080)
go run ./cmd/webcrawler --config config.yaml
```

---

## API Overview

| Method | Path                        | Description                                                          |
|--------|-----------------------------|----------------------------------------------------------------------|
| POST   | `/v1/jobs/custom`           | Submit a custom job with explicit URLs and crawl parameters.         |
| POST   | `/v1/jobs/standard`         | Submit a templated job by name (uses `standard_jobs` config).        |
| GET    | `/v1/jobs/{job_id}/status`  | Retrieve current job status, counters, and timestamps.               |
| GET    | `/v1/jobs/{job_id}/result`  | Fetch page-level metadata (URLs, hashes, headless flag, blob URIs).  |
| POST   | `/v1/jobs/{job_id}/cancel`  | Attempt to cancel an in-flight job.                                  |
| GET    | `/healthz`, `/readyz`       | Liveness/readiness checks.                                           |
| GET    | `/metrics`                  | Prometheus metrics (HTTP, queue depth, job/page counters).           |

Custom job request example:

```json
{
  "urls": ["https://example.com"],
  "max_depth": 1,
  "max_pages": 10,
  "headless_allowed": true,
  "respect_robots": true,
  "tags": {
    "intent": "price-refresh"
  }
}
```

---

## Configuration Highlights

Configuration is merged from `config.yaml` and `CRAWLER_*` environment variables. Key sections:

- `server.port` – HTTP listen port.
- `auth.enabled`/`auth.api_key` – toggle API key middleware.
- `crawler.concurrency`, `crawler.queue_depth`, `crawler.user_agent`, `crawler.max_depth_default`, `crawler.max_pages_default`.
- `http.timeout_seconds` – per-page probe budget (also used as default job budget).
- `headless.enabled`, `headless.max_parallel`, `headless.nav_timeout_seconds`, `headless.promotion_threshold`.
- `storage.backend` (`memory` or `gcs`), `storage.bucket` (required for `gcs`), `storage.prefix`, `storage.content_type`.
- `pubsub.topic_name`.
- `logging.development` – toggles zap dev vs prod logger.
- `standard_jobs.<name>` – canned job parameter templates.

See [`INSTRUCTIONS.md`](INSTRUCTIONS.md) for detailed operational guidance.

---

## Development Notes

- Make targets are replaced by Go tooling + pre-commit hooks (`gofumpt`, `goimports`, `golangci-lint`, `go test`, `govulncheck`).
- Memory-backed `JobStore` and `Publisher` keep the binary dependency-free; swap in the provided GCS BlobStore and real queues/databases as needed.
- The queue is currently in-memory FIFO; replace with a priority-aware implementation to enforce domain budgets or SLA tiers.
- Zap logging defaults to development mode with colorized output; production mode uses JSON with timestamps and includes stack traces.

---

## Roadmap

- Integrate real blob storage (GCS), relational DB, and Pub/Sub clients.
- Expand policy engine (robots, per-domain backoff, depth/budget enforcement).
- Publish Prometheus metrics/OTEL traces and add structured error codes to API responses.
- Add integration tests using emulators for storage/pubsub.

---

For more depth—including deployment, API payload details, observability, and security—read the [INSTRUCTIONS.md](INSTRUCTIONS.md).

---

## Storage Layout

When `storage.backend` is `gcs`, raw crawl artifacts are persisted as:

```
gs://<bucket>/<prefix>/yyyymm/dd/hh/host=<host>/id=<uuid7>/
  raw.html
  headers.json
  meta.json        # {url, status, size, sha256, content_type, parent_id, job_uuid}
  screenshot.png   # optional
```

Each fetched page receives its own UUIDv7 (also recorded in the API response) so the files can be referenced from downstream databases or workers.
