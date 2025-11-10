package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

type exampleProgressRepo struct {
	jobs []store.JobRun
}

func (e *exampleProgressRepo) UpsertJobStart(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (e *exampleProgressRepo) CompleteJob(context.Context, uuid.UUID, time.Time, store.JobRunStatus, *string) error {
	return nil
}

func (e *exampleProgressRepo) UpsertSiteStats(
	context.Context,
	uuid.UUID,
	string,
	int64,
	int64,
	string,
	time.Time,
) error {
	return nil
}

func (e *exampleProgressRepo) GetJob(context.Context, uuid.UUID) (store.JobRun, error) {
	return e.jobs[0], nil
}

func (e *exampleProgressRepo) ListJobs(context.Context, *store.JobRunStatus, int, int) ([]store.JobRun, error) {
	return e.jobs, nil
}

func (e *exampleProgressRepo) ListJobSites(context.Context, uuid.UUID, int, int) ([]store.SiteStats, error) {
	return nil, nil
}

// ExampleProgressHandler_ListJobs shows how to serve the /api/jobs endpoint.
func ExampleProgressHandler_ListJobs() {
	jobID := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	repo := &exampleProgressRepo{
		jobs: []store.JobRun{{
			ID:        jobID,
			JobID:     jobID,
			Status:    store.RunSuccess,
			StartedAt: time.Unix(0, 0),
		}},
	}
	handler := NewProgressHandler(repo, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=1", nil)
	rec := httptest.NewRecorder()
	handler.ListJobs(rec, req)

	var payload struct {
		Jobs []map[string]any `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		panic(err)
	}
	fmt.Printf("returned jobs: %d\n", len(payload.Jobs))
	// Output:
	// returned jobs: 1
}
