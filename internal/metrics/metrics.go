// Package metrics exposes Prometheus collectors for the crawler service.
package metrics

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	crawlerPagesTotal                    *prometheus.CounterVec
	crawlerBytesTotal                    *prometheus.CounterVec
	httpRequestsTotal                    *prometheus.CounterVec
	httpRequestDurationSeconds           *prometheus.HistogramVec
	crawlerProbeTLSHandshakeTimeoutTotal prometheus.Counter
	crawlerJobsTotal                     *prometheus.CounterVec
	crawlerActiveWorkers                 prometheus.Gauge
	crawlerRateLimitDelaysSeconds        *prometheus.HistogramVec

	once sync.Once
)

// Init initializes the Prometheus metrics collectors.
// It is safe to call this function multiple times.
func Init() {
	once.Do(func() {
		crawlerPagesTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "crawler_pages_total",
				Help: "Total number of pages crawled, labeled by site and status.",
			},
			[]string{"site", "status"},
		)

		crawlerBytesTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "crawler_bytes_total",
				Help: "Total number of bytes fetched, labeled by site.",
			},
			[]string{"site"},
		)

		httpRequestsTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests, labeled by method and code.",
			},
			[]string{"method", "code"},
		)

		httpRequestDurationSeconds = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Histogram of HTTP request latencies, labeled by method and route.",
				Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5},
			},
			[]string{"method", "route"},
		)

		crawlerProbeTLSHandshakeTimeoutTotal = promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "crawler_probe_tls_handshake_timeout_total",
				Help: "Total TLS handshake timeouts encountered while probing robots.txt.",
			},
		)

		crawlerJobsTotal = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "crawler_jobs_total",
				Help: "Total number of jobs processed, labeled by status.",
			},
			[]string{"status"},
		)

		crawlerActiveWorkers = promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "crawler_active_workers",
				Help: "Number of workers currently processing a job.",
			},
		)

		crawlerRateLimitDelaysSeconds = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "crawler_rate_limit_delays_seconds",
				Help:    "Histogram of rate limit wait durations.",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"domain"},
		)
	})
}

// SanitizeSite sanitizes a URL to extract a lowercase hostname.
// It returns "unknown" if the URL is invalid.
func SanitizeSite(rawURL string) string {
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return "unknown"
	}
	return strings.ToLower(u.Hostname())
}

// Handler returns an http.Handler for exposing Prometheus metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveCrawl increments the crawler metrics.
func ObserveCrawl(site string, status string, bytesFetched int) {
	sanitizedSite := SanitizeSite(site)
	crawlerPagesTotal.WithLabelValues(sanitizedSite, status).Inc()
	if bytesFetched > 0 {
		crawlerBytesTotal.WithLabelValues(sanitizedSite).Add(float64(bytesFetched))
	}
}

// ObserveHTTPRequest increments the HTTP request metrics.
func ObserveHTTPRequest(method, route string, code int, duration time.Duration) {
	httpRequestsTotal.WithLabelValues(method, strconv.Itoa(code)).Inc()
	httpRequestDurationSeconds.WithLabelValues(method, route).Observe(duration.Seconds())
}

// ObserveProbeTLSHandshakeTimeout increments the probe-specific handshake timeout counter.
func ObserveProbeTLSHandshakeTimeout() {
	crawlerProbeTLSHandshakeTimeoutTotal.Inc()
}

// ObserveJob increments the job counter for the given status.
func ObserveJob(status string) {
	crawlerJobsTotal.WithLabelValues(status).Inc()
}

// IncActiveWorkers increments the active workers gauge.
func IncActiveWorkers() {
	crawlerActiveWorkers.Inc()
}

// DecActiveWorkers decrements the active workers gauge.
func DecActiveWorkers() {
	crawlerActiveWorkers.Dec()
}

// ObserveRateLimitDelay records the duration of a rate limit wait.
func ObserveRateLimitDelay(domain string, duration time.Duration) {
	crawlerRateLimitDelaysSeconds.WithLabelValues(domain).Observe(duration.Seconds())
}
