// Package api exposes the HTTP interface for the crawler service.
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/dispatcher"
)

// Server wires HTTP handlers to the dispatcher and stores.
type Server struct {
	router     chi.Router
	jobStore   crawler.JobStore
	dispatcher *dispatcher.Dispatcher
	idGen      crawler.IDGenerator
	clock      crawler.Clock
	cfg        config.Config
}

const metricsPayload = "# HELP webcrawler_build_info Build info\n" +
	"# TYPE webcrawler_build_info gauge\n" +
	"webcrawler_build_info 1\n"

// NewServer constructs a Server with middleware and routes.
func NewServer(
	jobStore crawler.JobStore,
	dispatcher *dispatcher.Dispatcher,
	idGen crawler.IDGenerator,
	clock crawler.Clock,
	cfg config.Config,
) *Server {
	s := &Server{
		jobStore:   jobStore,
		dispatcher: dispatcher,
		idGen:      idGen,
		clock:      clock,
		cfg:        cfg,
	}
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Use(loggingMiddleware)
	r.Use(recoverMiddleware)
	r.Use(timeoutMiddleware(60 * time.Second))
	if cfg.Auth.Enabled {
		r.Use(apiKeyMiddleware(cfg.Auth.APIKey))
	}

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Get("/metrics", s.metrics)

	r.Route("/v1", func(r chi.Router) {
		r.Route("/jobs", func(r chi.Router) {
			r.Post("/custom", s.submitCustomJob)
			r.Post("/standard", s.submitStandardJob)
			r.Route("/{job_id}", func(r chi.Router) {
				r.Get("/status", s.getJobStatus)
				r.Get("/result", s.getJobResult)
				r.Post("/cancel", s.cancelJob)
			})
		})
	})

	s.router = r
	return s
}

// Handler returns the Router for use with http.Server.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	// In-memory dependencies are always ready; in future check downstreams.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	// Placeholder metrics endpoint; wire Prometheus registry in future.
	w.Header().Set("Content-Type", "text/plain")
	if _, err := w.Write([]byte(metricsPayload)); err != nil {
		slog.Default().Error("metrics write failed", "error", err)
	}
}

func (s *Server) submitCustomJob(w http.ResponseWriter, r *http.Request) {
	var req customJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	params, err := s.toJobParameters(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobID, err := s.enqueueJob(r.Context(), params)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusRequestTimeout
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (s *Server) submitStandardJob(w http.ResponseWriter, r *http.Request) {
	var req standardJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing job name")
		return
	}
	templateParams, ok := s.cfg.StandardJobs[req.Name]
	if !ok {
		writeError(w, http.StatusNotFound, "standard job template not found")
		return
	}
	params := s.applyDefaults(cloneJobParameters(templateParams))
	jobID, err := s.enqueueJob(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

func (s *Server) getJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	job, err := s.jobStore.GetJob(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) getJobResult(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	job, err := s.jobStore.GetJob(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	pages, err := s.jobStore.ListPages(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch job pages")
		return
	}
	writeJSON(w, http.StatusOK, crawler.JobResult{Job: job, Pages: pages})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	if err := s.jobStore.UpdateJobStatus(
		r.Context(),
		jobID,
		crawler.JobStatusCanceled,
		"canceled via API",
		crawler.JobCounters{},
	); err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"job_id": jobID, "status": string(crawler.JobStatusCanceled)})
}

func (s *Server) enqueueJob(ctx context.Context, params crawler.JobParameters) (string, error) {
	if len(params.URLs) == 0 {
		return "", errors.New("at least one URL required")
	}
	jobID, err := s.idGen.NewID()
	if err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	now := s.clock.Now()
	job := crawler.Job{
		ID:         jobID,
		Status:     crawler.JobStatusQueued,
		Submitted:  now,
		Parameters: params,
		Counters:   crawler.JobCounters{},
	}
	if err := s.jobStore.CreateJob(ctx, job); err != nil {
		return "", fmt.Errorf("create job: %w", err)
	}
	queueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	item := crawler.QueueItem{
		JobID:     jobID,
		Params:    params,
		Attempt:   1,
		Submitted: now.Unix(),
	}
	if err := s.dispatcher.Enqueue(queueCtx, item); err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return jobID, nil
}

func (s *Server) toJobParameters(req customJobRequest) (crawler.JobParameters, error) {
	if len(req.URLs) == 0 {
		return crawler.JobParameters{}, errors.New("urls required")
	}
	params := crawler.JobParameters{
		URLs:          req.URLs,
		PerDomainCaps: req.PerDomainCaps,
		Tags:          req.Tags,
		AllowDomains:  req.AllowDomains,
		DenyDomains:   req.DenyDomains,
	}
	maxDepth := valueOrDefault(req.MaxDepth, s.cfg.Crawler.MaxDepthDefault)
	maxPages := valueOrDefault(req.MaxPages, s.cfg.Crawler.MaxPagesDefault)
	budget := valueOrDefault(req.BudgetSeconds, s.cfg.HTTP.TimeoutSeconds)
	headless := boolOrDefault(req.HeadlessAllowed, s.cfg.Headless.Enabled)
	respectRobots := boolOrDefault(req.RespectRobots, !s.cfg.Crawler.IgnoreRobots)

	params.MaxDepth = maxDepth
	params.MaxPages = maxPages
	params.BudgetSeconds = budget
	params.HeadlessAllowed = headless
	params.HeadlessProvided = req.HeadlessAllowed != nil
	params.RespectRobots = respectRobots
	params.RespectRobotsProvided = req.RespectRobots != nil

	params = s.applyDefaults(params)
	return params, nil
}

type standardJobRequest struct {
	Name string `json:"name"`
}

type customJobRequest struct {
	URLs            []string          `json:"urls"`
	MaxDepth        *int              `json:"max_depth"`
	MaxPages        *int              `json:"max_pages"`
	BudgetSeconds   *int              `json:"budget_seconds"`
	HeadlessAllowed *bool             `json:"headless_allowed"`
	RespectRobots   *bool             `json:"respect_robots"`
	PerDomainCaps   map[string]int    `json:"per_domain_caps"`
	Tags            map[string]string `json:"tags"`
	AllowDomains    []string          `json:"allow_domains"`
	DenyDomains     []string          `json:"deny_domains"`
}

func valueOrDefault[T any](ptr *T, def T) T {
	if ptr == nil {
		return def
	}
	return *ptr
}

func boolOrDefault(ptr *bool, def bool) bool {
	if ptr == nil {
		return def
	}
	return *ptr
}

func (s *Server) applyDefaults(params crawler.JobParameters) crawler.JobParameters {
	if params.MaxDepth == 0 {
		params.MaxDepth = s.cfg.Crawler.MaxDepthDefault
	}
	if params.MaxPages == 0 {
		params.MaxPages = s.cfg.Crawler.MaxPagesDefault
	}
	if params.BudgetSeconds == 0 {
		params.BudgetSeconds = s.cfg.HTTP.TimeoutSeconds
	}
	if !params.HeadlessProvided {
		params.HeadlessAllowed = s.cfg.Headless.Enabled
		params.HeadlessProvided = true
	}
	if !params.RespectRobotsProvided {
		params.RespectRobots = !s.cfg.Crawler.IgnoreRobots
		params.RespectRobotsProvided = true
	}
	if params.PerDomainCaps == nil {
		params.PerDomainCaps = map[string]int{}
	}
	if params.Tags == nil {
		params.Tags = map[string]string{}
	}
	return params
}

func cloneJobParameters(src crawler.JobParameters) crawler.JobParameters {
	cp := src
	cp.URLs = cloneStringSlice(src.URLs)
	cp.AllowDomains = cloneStringSlice(src.AllowDomains)
	cp.DenyDomains = cloneStringSlice(src.DenyDomains)
	if src.PerDomainCaps != nil {
		cp.PerDomainCaps = make(map[string]int, len(src.PerDomainCaps))
		for k, v := range src.PerDomainCaps {
			cp.PerDomainCaps[k] = v
		}
	}
	if src.Tags != nil {
		cp.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			cp.Tags[k] = v
		}
	}
	return cp
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.NewString()
		ctx := context.WithValue(r.Context(), requestIDKey{}, reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "error", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func timeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "request timed out")
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		conn, buf, err := h.Hijack()
		if err != nil {
			return nil, nil, fmt.Errorf("hijack connection: %w", err)
		}
		return conn, buf, nil
	}
	return nil, nil, errors.New("hijacker not supported")
}

type requestIDKey struct{}

func apiKeyMiddleware(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if key != expected {
				writeError(w, http.StatusForbidden, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Default().Error("write JSON failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
