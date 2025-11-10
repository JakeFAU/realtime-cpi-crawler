package sinks

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
)

// PrometheusSink exports crawler progress metrics via Prometheus. It owns all
// collectors for jobs started/completed/running and per-site fetch counters.
type PrometheusSink struct {
	jobsStarted   prometheus.Counter
	jobsCompleted *prometheus.CounterVec
	jobsRunning   prometheus.Gauge
	jobRuntime    *prometheus.HistogramVec

	fetchRequests *prometheus.CounterVec
	fetchBytes    *prometheus.CounterVec
	fetchDuration *prometheus.HistogramVec

	tracker *jobTracker
}

// NewPrometheusSink registers the collectors against the provided registry.
func NewPrometheusSink(reg prometheus.Registerer) (*PrometheusSink, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	s := &PrometheusSink{
		jobsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "crawler_jobs_started_total",
			Help: "Total jobs that have started.",
		}),
		jobsCompleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crawler_jobs_completed_total",
			Help: "Total jobs completed partitioned by result.",
		}, []string{"result"}),
		jobsRunning: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "crawler_jobs_running",
			Help: "Current number of running jobs.",
		}),
		jobRuntime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "crawler_job_runtime_seconds",
			Help:    "Wall time per completed job.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200},
		}, []string{"result"}),
		fetchRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crawler_fetch_requests_total",
			Help: "Fetch completions partitioned by site and status class.",
		}, []string{"site", "status_class"}),
		fetchBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crawler_fetch_bytes_total",
			Help: "Bytes downloaded per site.",
		}, []string{"site"}),
		fetchDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "crawler_fetch_duration_seconds",
			Help:    "Fetch duration partitioned by site and status class.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
		}, []string{"site", "status_class"}),
		tracker: newJobTracker(),
	}
	for _, collector := range []prometheus.Collector{
		s.jobsStarted,
		s.jobsCompleted,
		s.jobsRunning,
		s.jobRuntime,
		s.fetchRequests,
		s.fetchBytes,
		s.fetchDuration,
	} {
		if err := reg.Register(collector); err != nil {
			return nil, fmt.Errorf("register progress collector: %w", err)
		}
	}
	return s, nil
}

// Consume updates the Prometheus collectors using the provided batch. It is
// safe for concurrent use by multiple goroutines.
func (s *PrometheusSink) Consume(_ context.Context, batch []progress.Event) error {
	for _, evt := range batch {
		s.consumeEvent(evt)
	}
	return nil
}

func (s *PrometheusSink) consumeEvent(evt progress.Event) {
	switch evt.Stage {
	case progress.StageJobStart, progress.StageJobDone, progress.StageJobError:
		s.handleJobEvent(evt)
	case progress.StageFetchDone:
		s.handleFetchEvent(evt)
	}
}

func (s *PrometheusSink) handleJobEvent(evt progress.Event) {
	switch evt.Stage {
	case progress.StageJobStart:
		s.jobsStarted.Inc()
		if s.tracker.start(evt.JobID) {
			s.jobsRunning.Inc()
		}
	case progress.StageJobDone:
		s.jobsCompleted.WithLabelValues("success").Inc()
		s.observeRuntime(evt, "success")
	case progress.StageJobError:
		s.jobsCompleted.WithLabelValues("error").Inc()
		s.observeRuntime(evt, "error")
	}
	if evt.Stage != progress.StageJobStart && s.tracker.complete(evt.JobID) {
		s.jobsRunning.Dec()
	}
}

func (s *PrometheusSink) observeRuntime(evt progress.Event, label string) {
	if evt.Dur > 0 {
		s.jobRuntime.WithLabelValues(label).Observe(evt.Dur.Seconds())
	}
}

func (s *PrometheusSink) handleFetchEvent(evt progress.Event) {
	site := evt.Site
	if site == "" {
		site = "unknown"
	}
	statusClass := string(evt.StatusClass)
	if statusClass == "" {
		statusClass = string(progress.StatusOther)
	}
	s.fetchRequests.WithLabelValues(site, statusClass).Inc()
	if evt.Bytes > 0 {
		s.fetchBytes.WithLabelValues(site).Add(float64(evt.Bytes))
	}
	if evt.Dur > 0 {
		s.fetchDuration.WithLabelValues(site, statusClass).Observe(evt.Dur.Seconds())
	}
}

// Close implements the Sink interface; it performs no action.
func (s *PrometheusSink) Close(context.Context) error {
	return nil
}

type jobTracker struct {
	mu      sync.Mutex
	running map[[16]byte]struct{}
}

func newJobTracker() *jobTracker {
	return &jobTracker{running: make(map[[16]byte]struct{})}
}

func (t *jobTracker) start(id [16]byte) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.running[id]; ok {
		return false
	}
	t.running[id] = struct{}{}
	return true
}

func (t *jobTracker) complete(id [16]byte) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.running[id]; !ok {
		return false
	}
	delete(t.running, id)
	return true
}
