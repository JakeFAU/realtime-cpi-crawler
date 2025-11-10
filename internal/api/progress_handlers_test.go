package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

func TestProgressHandlerListJobs(t *testing.T) {
	t.Parallel()

	jobID := uuid.New()
	repo := &mockProgressRepo{
		jobs: []store.JobRun{
			{
				ID:        uuid.New(),
				JobID:     jobID,
				Status:    store.RunSuccess,
				StartedAt: time.Now().Add(-time.Hour),
			},
		},
	}
	handler := NewProgressHandler(repo, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/jobs?status=success&limit=10", nil)
	rec := httptest.NewRecorder()

	handler.ListJobs(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "jobs")
}

func TestProgressHandlerGetJobNotFound(t *testing.T) {
	t.Parallel()

	repo := &mockProgressRepo{err: store.ErrNotFound}
	handler := NewProgressHandler(repo, zap.NewNop())

	jobID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID.String(), nil)
	req = withJobIDParam(req, jobID.String())
	rec := httptest.NewRecorder()

	handler.GetJob(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProgressHandlerListJobSitesInvalidLimit(t *testing.T) {
	t.Parallel()

	handler := NewProgressHandler(&mockProgressRepo{}, zap.NewNop())
	jobID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID.String()+"/sites?limit=-1", nil)
	req = withJobIDParam(req, jobID.String())
	rec := httptest.NewRecorder()

	handler.ListJobSites(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

type mockProgressRepo struct {
	jobs  []store.JobRun
	sites []store.SiteStats
	err   error
}

func (m *mockProgressRepo) UpsertJobStart(context.Context, uuid.UUID, time.Time) error {
	return m.err
}

func (m *mockProgressRepo) CompleteJob(context.Context, uuid.UUID, time.Time, store.JobRunStatus, *string) error {
	return m.err
}

func (m *mockProgressRepo) UpsertSiteStats(context.Context, uuid.UUID, string, int64, int64, string, time.Time) error {
	return m.err
}

func (m *mockProgressRepo) GetJob(context.Context, uuid.UUID) (store.JobRun, error) {
	if len(m.jobs) > 0 {
		return m.jobs[0], nil
	}
	return store.JobRun{}, m.err
}

func (m *mockProgressRepo) ListJobs(context.Context, *store.JobRunStatus, int, int) ([]store.JobRun, error) {
	return m.jobs, m.err
}

func (m *mockProgressRepo) ListJobSites(context.Context, uuid.UUID, int, int) ([]store.SiteStats, error) {
	return m.sites, m.err
}

func withJobIDParam(r *http.Request, jobID string) *http.Request {
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("job_id", jobID)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
}
