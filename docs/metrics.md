# Metrics

The crawler service exports Prometheus metrics for monitoring and alerting.

## Exported Metrics

| Name                              | Type      | Labels                               | Description                                           |
| --------------------------------- | --------- | ------------------------------------ | ----------------------------------------------------- |
| `crawler_pages_total`             | `counter` | `site`, `status` (`success`\|`error`) | Total number of pages crawled.                        |
| `crawler_bytes_total`             | `counter` | `site`                               | Total bytes fetched from pages.                       |
| `http_requests_total`             | `counter` | `method`, `code`                     | Total number of HTTP requests handled by the API.     |
| `http_request_duration_seconds`   | `histogram` | `method`, `route`                    | Latency distribution of API requests.                 |

## Label Cardinality

To prevent unbounded cardinality, labels are sanitized:

*   `site`: The raw URL is sanitized to only include the lowercase hostname (e.g., `example.com`).
*   `job_id`: Not used as a label to avoid high cardinality.
*   `status`: A small, fixed set of values (`success`, `error`).
*   `code`: HTTP status codes (e.g., `200`, `404`). While there can be many, in practice the number of distinct codes is limited.
*   `route`: The `chi` route pattern (e.g., `/api/jobs/{job_id}`).

## Example PromQL Queries

### Throughput

```promql
# Pages crawled per second, by site
sum(rate(crawler_pages_total{status="success"}[5m])) by (site)
```

### Error Rate

```promql
# Percentage of failed crawls over the last 5 minutes
sum(rate(crawler_pages_total{status="error"}[5m])) / sum(rate(crawler_pages_total[5m]))
```

### P95 Latency

```promql
# 95th percentile latency for API requests
histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))
```

### Bytes/sec by Site

```promql
# Bytes fetched per second, by site
sum(rate(crawler_bytes_total[5m])) by (site)
```
