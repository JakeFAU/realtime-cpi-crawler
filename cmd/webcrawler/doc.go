// Package main hosts the crawler service entrypoint.
//
// Architecture overview:
//   - HTTP API: internal/api.Server exposes health, metrics, and job management endpoints. Requests are validated,
//     normalized into crawler.JobParameters, and persisted via the JobStore before being enqueued for work.
//   - Dispatcher & queue: jobs flow through a bounded in-memory queue sized by config.Crawler.QueueDepth and are
//     fanned out to a fixed worker pool sized by config.Crawler.Concurrency. Context cancellation stops workers
//     cleanly on shutdown.
//   - Fetch pipeline: workers perform a lightweight probe fetch via the Colly-based fetcher (with optional robots.txt
//     enforcement), optionally promote to a headless Chromedp fetch when the heuristic detector deems it necessary,
//     and attach basic headers/status metadata to each response.
//   - Persistence & fanout: raw HTML (and screenshots when present) are written to the configured BlobStore
//     (memory/local/GCS). Retrieval metadata is optionally persisted to Postgres, and a compact Pub/Sub notification is
//     published when a topic is configured. Progress events are buffered and sent to configured sinks for monitoring.
//   - Configuration & plumbing: Viper populates config from env/files; zap provides structured logging; Prometheus
//     metrics are exported via the metrics middleware and /metrics handler; policy hooks exist for future admission
//     control. The service is stateless across requests, suitable for Cloud Run scale-out.
//
// Operational notes:
//   - Concurrency model: bounded queue + fixed worker pool; headless fetches have their own semaphore inside the
//     Chromedp fetcher. Shutdown is coordinated via context cancellation propagated from main through dispatcher to
//     workers.
//   - Rate limiting/backoff: the Colly robots transport retries transient TLS handshake issues for robots.txt. No
//     per-domain throttling is currently enforced; apply policy implementations to tighten this when needed.
//   - Observability: zap logs carry job IDs and URLs at key transitions; Prometheus counters/histograms track API and
//     crawl activity; progress Hub batches crawl lifecycle events for downstream sinks. Tracing is not yet wired in.
//   - Cloud Run: the HTTP server listens on the configured port (overridable via PORT). Health endpoints (/healthz,
//     /readyz) remain lightweight; the process reacts to SIGTERM for graceful drain and shutdown of workers.
//
// Quick checklist:
//   - Configure env vars: CRAWLER_SERVER_PORT or PORT, CRAWLER_CRAWLER_CONCURRENCY, CRAWLER_HTTP_TIMEOUT_SECONDS,
//     CRAWLER_CRAWLER_IGNORE_ROBOTS=false/true, CRAWLER_HEADLESS_ENABLED, storage (CRAWLER_STORAGE_*), pubsub, and
//     database DSN/table names when persistence beyond memory is required.
//   - Run locally: go run ./cmd/webcrawler -config config.yaml (or rely solely on env overrides).
//   - Cloud Run: container listens on PORT, remains stateless across requests, and shuts down cleanly on SIGTERM with
//     in-flight work bounded by per-job timeouts.
package main
