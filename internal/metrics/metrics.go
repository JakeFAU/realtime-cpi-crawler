// Package metrics exposes Prometheus collectors for the crawler service.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

const namespace = "webcrawler"

// Collectors wires together the Prometheus registry and metric instruments.
type Collectors struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	queueDepth prometheus.Gauge
	queueOps   *prometheus.CounterVec

	jobsSubmitted *prometheus.CounterVec
	jobsFinished  *prometheus.CounterVec
	jobDuration   *prometheus.HistogramVec

	pagesProcessed *prometheus.CounterVec
	pageDuration   *prometheus.HistogramVec
}

// New builds a Collectors bundle with all instruments registered.
func New() *Collectors {
	reg := prometheus.NewRegistry()
	c := &Collectors{registry: reg}
	c.init()
	return c
}

// Registry exposes the underlying Prometheus registry.
func (c *Collectors) Registry() *prometheus.Registry {
	if c == nil {
		return nil
	}
	return c.registry
}

// Handler returns an http.Handler that exports the registry in Prometheus format.
func (c *Collectors) Handler() http.Handler {
	if c == nil || c.registry == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// ObserveHTTPRequest records the volume and latency for an API call.
func (c *Collectors) ObserveHTTPRequest(method, route string, status int, d time.Duration) {
	if c == nil {
		return
	}
	statusCode := strconv.Itoa(status)
	c.httpRequests.WithLabelValues(method, route, statusCode).Inc()
	c.httpDuration.WithLabelValues(method, route).Observe(d.Seconds())
}

// RecordQueueEvent updates queue depth and operation counters.
func (c *Collectors) RecordQueueEvent(operation string, depth int) {
	if c == nil {
		return
	}
	c.queueOps.WithLabelValues(operation).Inc()
	c.queueDepth.Set(float64(depth))
}

// JobSubmitted increments the submission counter for the provided type.
func (c *Collectors) JobSubmitted(jobType string) {
	if c == nil {
		return
	}
	c.jobsSubmitted.WithLabelValues(jobType).Inc()
}

// JobFinished captures terminal job state metrics.
func (c *Collectors) JobFinished(status crawler.JobStatus, duration time.Duration) {
	if c == nil {
		return
	}
	statusLabel := string(status)
	c.jobsFinished.WithLabelValues(statusLabel).Inc()
	c.jobDuration.WithLabelValues(statusLabel).Observe(duration.Seconds())
}

// PageProcessed records per-page outcomes including headless promotion usage.
func (c *Collectors) PageProcessed(success bool, usedHeadless bool, statusCode int, duration time.Duration) {
	if c == nil {
		return
	}
	outcome := "succeeded"
	if !success {
		outcome = "failed"
	}
	headless := strconv.FormatBool(usedHeadless)
	code := strconv.Itoa(statusCode)
	c.pagesProcessed.WithLabelValues(outcome, headless, code).Inc()
	if duration > 0 {
		c.pageDuration.WithLabelValues(headless).Observe(duration.Seconds())
	}
}

func (c *Collectors) init() {
	if c.registry == nil {
		c.registry = prometheus.NewRegistry()
	}
	constBuckets := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
	c.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Count of HTTP requests handled by method, route, and status.",
	}, []string{"method", "route", "status"})
	c.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "Latency distribution of HTTP requests.",
		Buckets:   constBuckets,
	}, []string{"method", "route"})

	c.queueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "queue",
		Name:      "depth",
		Help:      "Current number of jobs waiting in the queue.",
	})
	c.queueOps = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "queue",
		Name:      "operations_total",
		Help:      "Queue enqueue/dequeue operations.",
	}, []string{"operation"})

	c.jobsSubmitted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "jobs",
		Name:      "submitted_total",
		Help:      "Jobs accepted by type.",
	}, []string{"type"})
	c.jobsFinished = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "jobs",
		Name:      "finished_total",
		Help:      "Jobs finished by final status.",
	}, []string{"status"})
	c.jobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "jobs",
		Name:      "duration_seconds",
		Help:      "End-to-end wall clock duration per job.",
		Buckets:   []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200},
	}, []string{"status"})

	c.pagesProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "pages",
		Name:      "processed_total",
		Help:      "Page processing outcomes split by success and headless promotion.",
	}, []string{"outcome", "headless", "status"})
	c.pageDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Subsystem: "pages",
		Name:      "duration_seconds",
		Help:      "Duration of page fetches split by promotion path.",
		Buckets:   constBuckets,
	}, []string{"headless"})

	c.registry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		c.httpRequests,
		c.httpDuration,
		c.queueDepth,
		c.queueOps,
		c.jobsSubmitted,
		c.jobsFinished,
		c.jobDuration,
		c.pagesProcessed,
		c.pageDuration,
	)
}
