// Package api hosts the HTTP server, middleware, and REST handlers for operator
// access. Notable routes:
//   - GET /healthz / readyz for Kubernetes probes.
//   - GET /metrics for Prometheus scraping.
//   - POST /v1/jobs/... for job submission/cancellation.
//   - GET /api/jobs and /api/jobs/{id}/sites for progress reporting via the
//     ProgressRepository interface.
package api
