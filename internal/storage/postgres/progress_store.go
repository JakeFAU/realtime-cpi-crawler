// Package postgres provides Postgres-backed persistence implementations.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

// ProgressStore implements the store.ProgressRepository interface using Postgres.
type ProgressStore struct {
	pool *pgxpool.Pool
}

// NewProgressStore creates a new ProgressStore.
func NewProgressStore(ctx context.Context, dsn string) (*ProgressStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}
	return &ProgressStore{pool: pool}, nil
}

// Close closes the underlying connection pool.
func (s *ProgressStore) Close() {
	s.pool.Close()
}

// UpsertJobStart inserts or updates a job's start time.
func (s *ProgressStore) UpsertJobStart(ctx context.Context, jobID uuid.UUID, startedAt time.Time) error {
	query := `
		INSERT INTO job_runs (id, job_id, started_at, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id, started_at) DO UPDATE
		SET status = EXCLUDED.status
		WHERE job_runs.status <> EXCLUDED.status;
	`
	_, err := s.pool.Exec(ctx, query, jobID, jobID, startedAt, store.RunRunning)
	if err != nil {
		return fmt.Errorf("failed to upsert job start: %w", err)
	}
	return nil
}

// CompleteJob marks a job as completed with a status and optional error message.
func (s *ProgressStore) CompleteJob(
	ctx context.Context,
	jobID uuid.UUID,
	finishedAt time.Time,
	status store.JobRunStatus,
	errMsg *string,
) error {
	query := `
		UPDATE job_runs
		SET finished_at = $1, status = $2, error_message = $3
		WHERE id = $4;
	`
	_, err := s.pool.Exec(ctx, query, finishedAt, status, errMsg, jobID)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}
	return nil
}

// UpsertSiteStats updates the statistics for a given site within a job.
func (s *ProgressStore) UpsertSiteStats(
	ctx context.Context,
	jobID uuid.UUID,
	site string,
	deltaVisits,
	deltaBytes int64,
	statusClass string,
	at time.Time,
) error {
	var query string
	switch statusClass {
	case "2xx":
		query = `UPDATE site_stats SET visits = visits + $1,
			bytes_total = bytes_total + $2,
			fetch_2xx = fetch_2xx + $1,
			last_update = $3
			WHERE job_id = $4 AND site = $5;`
	case "3xx":
		query = `UPDATE site_stats SET visits = visits + $1,
			bytes_total = bytes_total + $2,
			fetch_3xx = fetch_3xx + $1,
			last_update = $3
			WHERE job_id = $4 AND site = $5;`
	case "4xx":
		query = `UPDATE site_stats SET visits = visits + $1,
			bytes_total = bytes_total + $2,
			fetch_4xx = fetch_4xx + $1,
			last_update = $3
			WHERE job_id = $4 AND site = $5;`
	case "5xx":
		query = `UPDATE site_stats SET visits = visits + $1,
			bytes_total = bytes_total + $2,
			fetch_5xx = fetch_5xx + $1,
			last_update = $3
			WHERE job_id = $4 AND site = $5;`
	default:
		return fmt.Errorf("unknown status class: %s", statusClass)
	}

	res, err := s.pool.Exec(ctx, query, deltaVisits, deltaBytes, at, jobID, site)
	if err != nil {
		return fmt.Errorf("failed to update site stats: %w", err)
	}
	if res.RowsAffected() == 0 {
		var fetch2xx, fetch3xx, fetch4xx, fetch5xx int64
		switch statusClass {
		case "2xx":
			fetch2xx = deltaVisits
		case "3xx":
			fetch3xx = deltaVisits
		case "4xx":
			fetch4xx = deltaVisits
		case "5xx":
			fetch5xx = deltaVisits
		}

		query = `
			INSERT INTO site_stats (job_id, site, last_update, visits, bytes_total, fetch_2xx, fetch_3xx, fetch_4xx, fetch_5xx)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (job_id, site) DO NOTHING;
		`
		_, err = s.pool.Exec(
			ctx,
			query,
			jobID,
			site,
			at,
			deltaVisits,
			deltaBytes,
			fetch2xx,
			fetch3xx,
			fetch4xx,
			fetch5xx,
		)
		if err != nil {
			return fmt.Errorf("failed to insert site stats: %w", err)
		}
	}
	return nil
}

// GetJob retrieves a single job run by its ID.
func (s *ProgressStore) GetJob(ctx context.Context, jobID uuid.UUID) (store.JobRun, error) {
	query := `
		SELECT id, job_id, started_at, finished_at, status, error_message
		FROM job_runs
		WHERE id = $1;
	`
	var run store.JobRun
	err := s.pool.QueryRow(ctx, query, jobID).Scan(
		&run.ID,
		&run.JobID,
		&run.StartedAt,
		&run.FinishedAt,
		&run.Status,
		&run.ErrorMessage,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.JobRun{}, store.ErrNotFound
		}
		return store.JobRun{}, fmt.Errorf("failed to get job: %w", err)
	}
	return run, nil
}

// ListJobs retrieves a list of job runs, with optional status filtering.
func (s *ProgressStore) ListJobs(
	ctx context.Context,
	status *store.JobRunStatus,
	limit,
	offset int,
) ([]store.JobRun, error) {
	query := `
		SELECT id, job_id, started_at, finished_at, status, error_message
		FROM job_runs
		WHERE ($1::text IS NULL OR status = $1)
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3;
	`
	rows, err := s.pool.Query(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer rows.Close()

	var runs []store.JobRun
	for rows.Next() {
		var run store.JobRun
		err := rows.Scan(
			&run.ID,
			&run.JobID,
			&run.StartedAt,
			&run.FinishedAt,
			&run.Status,
			&run.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan job row: %w", err)
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// ListJobSites retrieves aggregated site statistics for a given job.
func (s *ProgressStore) ListJobSites(
	ctx context.Context,
	jobID uuid.UUID,
	limit,
	offset int,
) ([]store.SiteStats, error) {
	query := `
		SELECT job_id, site, last_update, visits, bytes_total, fetch_2xx, fetch_3xx, fetch_4xx, fetch_5xx
		FROM site_stats
		WHERE job_id = $1
		ORDER BY last_update DESC
		LIMIT $2 OFFSET $3;
	`
	rows, err := s.pool.Query(ctx, query, jobID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list job sites: %w", err)
	}
	defer rows.Close()

	var stats []store.SiteStats
	for rows.Next() {
		var stat store.SiteStats
		err := rows.Scan(
			&stat.JobID,
			&stat.Site,
			&stat.LastUpdate,
			&stat.Visits,
			&stat.BytesTotal,
			&stat.Fetch2xx,
			&stat.Fetch3xx,
			&stat.Fetch4xx,
			&stat.Fetch5xx,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site stats row: %w", err)
		}
		stats = append(stats, stat)
	}
	return stats, nil
}
