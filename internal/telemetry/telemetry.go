// Package telemetry unifies OpenTelemetry tracing (Google Cloud) and Prometheus metrics.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// --- CUSTOM METRIC DEFINITIONS ---

var (
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
)

var (
	initOnce  sync.Once
	traceProv *sdktrace.TracerProvider
	meterProv *metric.MeterProvider
	initErr   error
)

// --- INITIALIZATION ---

// InitTelemetry sets up Tracing (Google Cloud) and Metrics (Prometheus Sidecar).
func InitTelemetry(ctx context.Context, cfg *config.Config) (*sdktrace.TracerProvider, *metric.MeterProvider, error) {
	initOnce.Do(func() {
		// 1. Define Resource
		res, err := resource.New(ctx,
			resource.WithAttributes(
				semconv.ServiceName(cfg.Application.ServiceName),
				semconv.ServiceVersion(cfg.Application.Version),
				semconv.CloudAccountID(cfg.Application.ProjectNumber),
				semconv.CloudRegion(cfg.Application.Region),
				semconv.CloudProviderGCP,
				semconv.CloudPlatformGCPCloudRun,
			),
		)
		if err != nil {
			initErr = fmt.Errorf("failed to create resource: %w", err)
			return
		}

		// 2. Setup TRACING (Direct export to Google Cloud Trace)
		var traceExporter sdktrace.SpanExporter
		if cfg.Application.ProjectID != "" {
			traceExporter, err = texporter.New(texporter.WithProjectID(cfg.Application.ProjectID))
			if err != nil {
				initErr = fmt.Errorf("failed to create google trace exporter: %w", err)
				return
			}
		}

		opts := []sdktrace.TracerProviderOption{
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		}
		if traceExporter != nil {
			opts = append(opts, sdktrace.WithBatcher(traceExporter))
		}

		tp := sdktrace.NewTracerProvider(opts...)
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
		)

		// 3. Setup METRICS (Bridge OpenTelemetry to the existing Prometheus Registry)
		// This ensures OTel metrics AND your custom 'crawlerPagesTotal' vars appear on the same endpoint.
		promExporter, err := otelprom.New(
			otelprom.WithRegisterer(prometheus.DefaultRegisterer), // <--- KEY: Use the same registry as promauto
		)
		if err != nil {
			initErr = fmt.Errorf("failed to create prometheus exporter: %w", err)
			return
		}

		mp := metric.NewMeterProvider(
			metric.WithResource(res),
			metric.WithReader(promExporter),
		)
		otel.SetMeterProvider(mp)
		traceProv = tp
		meterProv = mp
	})
	return traceProv, meterProv, initErr
}

// --- HTTP HANDLER & MIDDLEWARE ---

// Handler returns the standard Prometheus HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// Middleware is a chi middleware that records HTTP request metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)

		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = "unknown"
		}

		// Call the function directly (no package prefix needed here)
		ObserveHTTPRequest(r.Method, routePattern, ww.statusCode, time.Since(start))
	})
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

// --- HELPER FUNCTIONS ---

// SanitizeSite extracts the hostname from a URL.
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

// ObserveCrawl records metrics for a crawled page.
func ObserveCrawl(site string, status string, bytesFetched int) {
	sanitizedSite := SanitizeSite(site)
	crawlerPagesTotal.WithLabelValues(sanitizedSite, status).Inc()
	if bytesFetched > 0 {
		crawlerBytesTotal.WithLabelValues(sanitizedSite).Add(float64(bytesFetched))
	}
}

// ObserveHTTPRequest records metrics for an HTTP request.
func ObserveHTTPRequest(method, route string, code int, duration time.Duration) {
	httpRequestsTotal.WithLabelValues(method, strconv.Itoa(code)).Inc()
	httpRequestDurationSeconds.WithLabelValues(method, route).Observe(duration.Seconds())
}

// ObserveProbeTLSHandshakeTimeout records a TLS handshake timeout during robots.txt probing.
func ObserveProbeTLSHandshakeTimeout() {
	crawlerProbeTLSHandshakeTimeoutTotal.Inc()
}

// ObserveJob records metrics for a job status change.
func ObserveJob(status string) {
	crawlerJobsTotal.WithLabelValues(status).Inc()
}

// IncActiveWorkers increments the active worker count.
func IncActiveWorkers() {
	crawlerActiveWorkers.Inc()
}

// DecActiveWorkers decrements the active worker count.
func DecActiveWorkers() {
	crawlerActiveWorkers.Dec()
}

// ObserveRateLimitDelay records the duration of a rate limit wait.
func ObserveRateLimitDelay(domain string, duration time.Duration) {
	crawlerRateLimitDelaysSeconds.WithLabelValues(domain).Observe(duration.Seconds())
}
