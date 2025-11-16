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
| GET    | `/api/jobs`                 | List job runs with optional status filtering and pagination.         |
| GET    | `/api/jobs/{job_id}`        | Get details of a specific job run.                                   |
| GET    | `/api/jobs/{job_id}/sites`  | List aggregated site statistics for a job (visits, bytes, status).   |
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
- `storage.backend` (`memory`, `gcs`, or `local`), `storage.bucket` (required for `gcs`), `storage.prefix`, `storage.content_type`, `storage.local.base_dir` (required for `local`).
- `database.host`, `database.port`, `database.user`, `database.password`, `database.name` (or the legacy `database.dsn`), `database.retrieval_table`, `database.progress_table`, `database.stats_table`, `database.max_conns`, `database.min_conns`, `database.max_conn_lifetime`.
- `pubsub.topic_name`, `pubsub.project_id`.
- `logging.development` – toggles zap dev vs prod logger.
- `metrics.enabled`, `metrics.path` – control Prometheus metrics exposition.
- `progress.enabled`, `progress.buffer_size`, `progress.batch.max_events`, `progress.batch.max_wait_ms`, `progress.sink_timeout_ms`, `progress.log_enabled` – control progress tracking subsystem.
- `standard_jobs.<name>` – canned job parameter templates.

See [`INSTRUCTIONS.md`](INSTRUCTIONS.md) for detailed operational guidance.

---

## Development Notes

- Make targets are replaced by Go tooling + pre-commit hooks (`gofumpt`, `goimports`, `golangci-lint`, `go test`, `govulncheck`).
- Memory-backed `JobStore` and `Publisher` keep the binary dependency-free; swap in the provided GCS/Cloud Pub/Sub/Postgres implementations as needed.
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

When `storage.backend` is `gcs` or `local`, raw crawl artifacts are persisted as:

```
gs://<bucket>/<prefix>/yyyymm/dd/hh/host=<host>/id=<uuid7>/   # or local path for 'local' backend
  raw.html
  headers.json
  meta.json        # {url, status, size, sha256, content_type, parent_id, job_uuid}
  screenshot.png   # optional
```

Each fetched page receives its own UUIDv7 (also recorded in the API response) so the files can be referenced from downstream databases or workers.

### Database Schema

When database connectivity is configured (via `database.dsn` or the host/user/password fields), the crawler persists data to three Postgres tables:

**Retrievals Table** (default: `crawl`, configured via `database.retrieval_table`):
- `id UUID PRIMARY KEY` – page UUID (matches blob path).
- `job_id UUID NOT NULL` – ID of the crawl job.
- `job_started_at TIMESTAMPTZ NOT NULL` – when the job began.
- `partition_ts TIMESTAMPTZ NOT NULL` – hour bucket (`retrieval_timestamp` truncated to the hour) to aid partitioning.
- `retrieval_timestamp TIMESTAMPTZ NOT NULL DEFAULT now()` – precise ingest time.
- `retrieval_url TEXT NOT NULL` – final URL fetched.
- `retrieval_hashcode TEXT` – SHA256/content hash.
- `retrieval_blob_location TEXT` – blob URI of `raw.html`.
- `retrieval_headers JSONB` – serialized HTTP headers.
- `retrieval_status_code INTEGER` – HTTP status.
- `retrieval_content_type TEXT` – MIME type inferred from the response/body.
- `parent_id UUID NULL` / `parent_ts TIMESTAMPTZ NULL` – optional ancestry for promoted/sub-pages.
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()` / `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`.

**Job Runs Table** (default: `job_runs`, configured via `database.progress_table`):
- `id UUID PRIMARY KEY` – run identifier (same as job_id).
- `job_id UUID NOT NULL` – job identifier.
- `started_at TIMESTAMPTZ NOT NULL` – when the job started.
- `finished_at TIMESTAMPTZ` – when the job completed.
- `status TEXT NOT NULL` – job status (running, success, error).
- `error_message TEXT` – error details if status is error.

**Site Stats Table** (default: `job_stats`, configured via `database.stats_table`):
- `job_id UUID NOT NULL` – associated job.
- `site TEXT NOT NULL` – domain/site being crawled.
- `last_update TIMESTAMPTZ NOT NULL` – last time stats were updated.
- `visits INT64 NOT NULL DEFAULT 0` – total page visits.
- `bytes_total INT64 NOT NULL DEFAULT 0` – total bytes fetched.
- `fetch_2xx INT64 NOT NULL DEFAULT 0` – count of 2xx responses.
- `fetch_3xx INT64 NOT NULL DEFAULT 0` – count of 3xx responses.
- `fetch_4xx INT64 NOT NULL DEFAULT 0` – count of 4xx responses.
- `fetch_5xx INT64 NOT NULL DEFAULT 0` – count of 5xx responses.

The worker only performs inserts; downstream consumers own any additional processing flags.

When both `pubsub.project_id` and `pubsub.topic_name` are provided, each successful fetch is published to Google Cloud Pub/Sub as a compact JSON document tailored for the AI stack handoff:

```json
{
  "crawl_id": "job-uuid",
  "job_id": "external-job-uuid",
  "site": "example.com",
  "url": "https://example.com/page",
  "html_blob": "gs://bucket/…/raw.html",
  "meta_blob": "gs://bucket/…/meta.json",
  "schema_version": "1"
}
```

`html_blob` contains the raw HTML, while `meta_blob` references the metadata JSON written alongside the HTML (status, size, hash, etc.). `job_id` mirrors the upstream caller supplied identifier (today equivalent to `crawl_id`), `url` captures the fully qualified page, and `schema_version` aids backwards-compatible migrations. This allows an AI post-processor to pick up work as soon as the retrieval lands without polling Postgres.
