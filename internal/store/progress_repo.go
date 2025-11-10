// Package store declares interfaces for persisting job progress.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound signals that the requested record does not exist.
var ErrNotFound = errors.New("progress record not found")

// JobRunStatus mirrors the job_runs status column.
type JobRunStatus string

// Job run statuses persisted in job_runs.status.
const (
	RunRunning JobRunStatus = "running"
	RunSuccess JobRunStatus = "success"
	RunError   JobRunStatus = "error"
)

// JobRun models the job_runs table for API responses.
type JobRun struct {
	// ID is the primary key of job_runs (may match JobID depending on schema).
	ID uuid.UUID
	// JobID is the logical crawl identifier shared with workers.
	JobID uuid.UUID
	// StartedAt captures when the run was first marked running.
	StartedAt time.Time
	// FinishedAt is nil until the run is marked success/error.
	FinishedAt *time.Time
	// Status is running/success/error.
	Status JobRunStatus
	// ErrorMessage optionally stores the final failure reason.
	ErrorMessage *string
}

// SiteStats captures per-site aggregation for a job.
type SiteStats struct {
	// JobID is the owning crawl.
	JobID uuid.UUID
	// Site is the normalized host label (e.g., example.com).
	Site string
	// LastUpdate captures the timestamp of the most recent aggregate.
	LastUpdate time.Time
	// Visits counts completed pages for the site.
	Visits int64
	// BytesTotal accumulates response bytes.
	BytesTotal int64
	// Fetch2xx-5xx hold per-status counts for diagnostics.
	Fetch2xx int64
	Fetch3xx int64
	Fetch4xx int64
	Fetch5xx int64
}

// ProgressRepository persists incremental job progress.
type ProgressRepository interface {
	// UpsertJobStart inserts (or idempotently updates) the started_at timestamp.
	UpsertJobStart(ctx context.Context, jobID uuid.UUID, startedAt time.Time) error
	// CompleteJob marks the run finished with the provided status and error.
	CompleteJob(ctx context.Context, jobID uuid.UUID, finishedAt time.Time, status JobRunStatus, errMsg *string) error
	// UpsertSiteStats applies visit/byte deltas per (job, site, statusClass).
	UpsertSiteStats(
		ctx context.Context,
		jobID uuid.UUID,
		site string,
		deltaVisits int64,
		deltaBytes int64,
		statusClass string,
		at time.Time,
	) error

	// GetJob loads a single job run or returns ErrNotFound.
	GetJob(ctx context.Context, jobID uuid.UUID) (JobRun, error)
	// ListJobs returns job runs filtered by optional status plus limit/offset.
	ListJobs(ctx context.Context, status *JobRunStatus, limit, offset int) ([]JobRun, error)
	// ListJobSites returns aggregated site stats for one job.
	ListJobSites(ctx context.Context, jobID uuid.UUID, limit, offset int) ([]SiteStats, error)
}
