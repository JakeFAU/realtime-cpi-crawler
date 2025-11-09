package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/dispatcher"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
)

func TestServer_SubmitCustomJob_Succeeds(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	q := queueMemory.NewQueue(10)
	dispatch := dispatcher.New(q, nil)
	idGen := &fakeIDGen{ids: []string{"job-custom"}}
	clock := &fakeClock{now: time.Unix(100, 0)}
	cfg := config.Config{
		Crawler: config.CrawlerConfig{
			MaxDepthDefault: 1,
			MaxPagesDefault: 10,
		},
		HTTP: config.HTTPConfig{
			TimeoutSeconds: 30,
		},
		Logging: config.LoggingConfig{Development: true},
	}
	server := NewServer(jobStore, dispatch, idGen, clock, cfg, zap.NewNop())

	reqBody := []byte(`{"urls":["https://example.com"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/custom", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Contains(t, rec.Body.String(), "job-custom")
	item, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "job-custom", item.JobID)
}

func TestServer_SubmitCustomJob_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/custom", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestServer_SubmitCustomJob_MissingURLs(t *testing.T) {
	t.Parallel()

	server := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/custom", bytes.NewBufferString(`{"urls":[]}`))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "urls required")
}

func TestServer_SubmitStandardJob_TemplateMissing(t *testing.T) {
	t.Parallel()

	svr := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/standard", bytes.NewBufferString(`{"name":"missing"}`))
	rec := httptest.NewRecorder()

	svr.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServer_GetJobStatus_ReturnsJob(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	jobStore.jobs["job-status"] = crawler.Job{ID: "job-status", Status: crawler.JobStatusSucceeded}
	server := newTestServerWithStore(jobStore)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job-status/status", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "succeeded")
}

func TestServer_GetJobResult_ReturnsPages(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	jobStore.jobs["job-result"] = crawler.Job{ID: "job-result", Status: crawler.JobStatusSucceeded}
	jobStore.pages["job-result"] = []crawler.PageRecord{
		{JobID: "job-result", URL: "https://example.com"},
	}
	server := newTestServerWithStore(jobStore)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job-result/result", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "example.com")
}

func TestServer_CancelJob_SetsStatusCanceled(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	jobStore.jobs["job-cancel"] = crawler.Job{ID: "job-cancel", Status: crawler.JobStatusRunning}
	server := newTestServerWithStore(jobStore)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-cancel/cancel", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, crawler.JobStatusCanceled, jobStore.lastStatus("job-cancel"))
}

func TestServer_SubmitStandardJob_Succeeds(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	q := queueMemory.NewQueue(10)
	dispatch := dispatcher.New(q, nil)
	cfg := config.Config{
		Crawler: config.CrawlerConfig{
			MaxDepthDefault: 1,
			MaxPagesDefault: 10,
		},
		HTTP: config.HTTPConfig{
			TimeoutSeconds: 30,
		},
		Logging: config.LoggingConfig{Development: true},
		StandardJobs: map[string]crawler.JobParameters{
			"price-refresh": {
				URLs:            []string{"https://example.com"},
				HeadlessAllowed: true,
			},
		},
	}
	server := NewServer(
		jobStore,
		dispatch,
		&fakeIDGen{ids: []string{"std-job"}},
		&fakeClock{now: time.Unix(50, 0)},
		cfg,
		zap.NewNop(),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/standard", bytes.NewBufferString(`{"name":"price-refresh"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	item, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "std-job", item.JobID)
}

func TestServer_GetJobResult_ListPagesError(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	jobStore.jobs["job"] = crawler.Job{ID: "job"}
	jobStore.listErr = errors.New("boom")
	server := newTestServerWithStore(jobStore)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job/result", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestServer_APIKeyMiddleware(t *testing.T) {
	t.Parallel()

	jobStore := newAPIFakeJobStore()
	q := queueMemory.NewQueue(1)
	dispatch := dispatcher.New(q, nil)
	cfg := config.Config{
		Crawler: config.CrawlerConfig{
			MaxDepthDefault: 1,
			MaxPagesDefault: 10,
		},
		HTTP: config.HTTPConfig{
			TimeoutSeconds: 30,
		},
		Logging: config.LoggingConfig{Development: true},
		Auth: config.AuthConfig{
			Enabled: true,
			APIKey:  "secret",
		},
	}
	server := NewServer(jobStore, dispatch, &fakeIDGen{}, &fakeClock{now: time.Unix(100, 0)}, cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-API-Key", "secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestIDMiddlewareSetsHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	newTestServer().Handler().ServeHTTP(rec, req)

	require.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestResponseWriterHijackBehavior(t *testing.T) {
	t.Parallel()

	rw := &responseWriter{ResponseWriter: httptest.NewRecorder()}
	if _, _, err := rw.Hijack(); err == nil || err.Error() != "hijacker not supported" {
		t.Fatalf("expected unsupported hijacker error, got %v", err)
	}

	h := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	rw = &responseWriter{ResponseWriter: h}
	conn, buf, err := rw.Hijack()
	if err != nil {
		t.Fatalf("expected successful hijack, got %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close hijacked conn: %v", err)
	}
	if err := h.CloseClient(); err != nil {
		t.Fatalf("close hijacked client: %v", err)
	}
	if buf == nil {
		t.Fatal("expected buf to be non-nil")
	}
}

// --- helpers/fakes ---

type fakeIDGen struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeIDGen) NewID() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ids) == 0 {
		return "id-default", nil
	}
	id := f.ids[0]
	f.ids = f.ids[1:]
	return id, nil
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

type apiJobStore struct {
	mu      sync.Mutex
	jobs    map[string]crawler.Job
	pages   map[string][]crawler.PageRecord
	listErr error
}

func newAPIFakeJobStore() *apiJobStore {
	return &apiJobStore{
		jobs:  make(map[string]crawler.Job),
		pages: make(map[string][]crawler.PageRecord),
	}
}

func (s *apiJobStore) CreateJob(_ context.Context, job crawler.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *apiJobStore) UpdateJobStatus(
	_ context.Context,
	jobID string,
	status crawler.JobStatus,
	errText string,
	counters crawler.JobCounters,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	job.Status = status
	job.ErrorText = errText
	job.Counters = counters
	s.jobs[jobID] = job
	return nil
}

func (s *apiJobStore) RecordPage(_ context.Context, _ crawler.PageRecord) error {
	return nil
}

func (s *apiJobStore) GetJob(_ context.Context, jobID string) (crawler.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return crawler.Job{}, errors.New("not found")
	}
	return job, nil
}

func (s *apiJobStore) ListPages(_ context.Context, jobID string) ([]crawler.PageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.pages[jobID], nil
}

func (s *apiJobStore) lastStatus(jobID string) crawler.JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[jobID].Status
}

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	client net.Conn
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	server, client := net.Pipe()
	h.client = client
	return server, bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client)), nil
}

func (h *hijackableRecorder) CloseClient() error {
	if h.client != nil {
		if err := h.client.Close(); err != nil {
			return fmt.Errorf("close hijacker client: %w", err)
		}
	}
	return nil
}

func newTestServer() *Server {
	return newTestServerWithStore(newAPIFakeJobStore())
}

func newTestServerWithStore(jobStore crawler.JobStore) *Server {
	q := queueMemory.NewQueue(10)
	dispatch := dispatcher.New(q, nil)
	cfg := config.Config{
		Crawler: config.CrawlerConfig{
			MaxDepthDefault: 1,
			MaxPagesDefault: 10,
		},
		HTTP: config.HTTPConfig{
			TimeoutSeconds: 30,
		},
		Logging: config.LoggingConfig{Development: true},
		StandardJobs: map[string]crawler.JobParameters{
			"price-refresh": {
				URLs:            []string{"https://example.com"},
				HeadlessAllowed: true,
			},
		},
	}
	return NewServer(
		jobStore,
		dispatch,
		&fakeIDGen{},
		&fakeClock{now: time.Unix(100, 0)},
		cfg,
		zap.NewNop(),
	)
}
