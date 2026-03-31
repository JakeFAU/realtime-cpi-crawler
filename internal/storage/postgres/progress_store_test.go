package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

func TestProgressStore_UpsertJobStart(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)

	jobID := uuid.New()
	startedAt := time.Now().UTC()

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO job_runs").
			WithArgs(jobID, jobID, startedAt, store.RunRunning).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		err = ps.UpsertJobStart(context.Background(), jobID, startedAt)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO job_runs").
			WithArgs(jobID, jobID, startedAt, store.RunRunning).
			WillReturnError(errors.New("db error"))

		err = ps.UpsertJobStart(context.Background(), jobID, startedAt)
		assert.ErrorContains(t, err, "failed to upsert job start")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProgressStore_CompleteJob(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)

	jobID := uuid.New()
	finishedAt := time.Now().UTC()
	status := store.RunSuccess
	errMsg := "some error"

	t.Run("success", func(t *testing.T) {
		mock.ExpectExec("UPDATE job_runs").
			WithArgs(finishedAt, status, &errMsg, jobID).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err = ps.CompleteJob(context.Background(), jobID, finishedAt, status, &errMsg)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectExec("UPDATE job_runs").
			WithArgs(finishedAt, status, &errMsg, jobID).
			WillReturnError(errors.New("db error"))

		err = ps.CompleteJob(context.Background(), jobID, finishedAt, status, &errMsg)
		assert.ErrorContains(t, err, "failed to complete job")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProgressStore_UpsertSiteStats(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)

	jobID := uuid.New()
	site := "example.com"
	at := time.Now().UTC()
	var deltaVisits, deltaBytes int64 = 1, 1024

	testCases := []struct {
		statusClass string
	}{
		{"2xx"},
		{"3xx"},
		{"4xx"},
		{"5xx"},
	}

	for _, tc := range testCases {
		t.Run("update existing "+tc.statusClass, func(t *testing.T) {
			mock.ExpectExec("UPDATE site_stats").
				WithArgs(deltaVisits, deltaBytes, at, jobID, site).
				WillReturnResult(pgxmock.NewResult("UPDATE", 1))

			err = ps.UpsertSiteStats(context.Background(), jobID, site, deltaVisits, deltaBytes, tc.statusClass, at)
			assert.NoError(t, err)
			assert.NoError(t, mock.ExpectationsWereMet())
		})

		t.Run("insert new "+tc.statusClass, func(t *testing.T) {
			mock.ExpectExec("UPDATE site_stats").
				WithArgs(deltaVisits, deltaBytes, at, jobID, site).
				WillReturnResult(pgxmock.NewResult("UPDATE", 0))

			var f2, f3, f4, f5 int64
			switch tc.statusClass {
			case "2xx":
				f2 = deltaVisits
			case "3xx":
				f3 = deltaVisits
			case "4xx":
				f4 = deltaVisits
			case "5xx":
				f5 = deltaVisits
			}

			mock.ExpectExec("INSERT INTO site_stats").
				WithArgs(jobID, site, at, deltaVisits, deltaBytes, f2, f3, f4, f5).
				WillReturnResult(pgxmock.NewResult("INSERT", 1))

			err = ps.UpsertSiteStats(context.Background(), jobID, site, deltaVisits, deltaBytes, tc.statusClass, at)
			assert.NoError(t, err)
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("unknown status class", func(t *testing.T) {
		err = ps.UpsertSiteStats(context.Background(), jobID, site, deltaVisits, deltaBytes, "6xx", at)
		assert.ErrorContains(t, err, "unknown status class: 6xx")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update error", func(t *testing.T) {
		mock.ExpectExec("UPDATE site_stats").
			WithArgs(deltaVisits, deltaBytes, at, jobID, site).
			WillReturnError(errors.New("db error"))

		err = ps.UpsertSiteStats(context.Background(), jobID, site, deltaVisits, deltaBytes, "2xx", at)
		assert.ErrorContains(t, err, "failed to update site stats")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("insert error", func(t *testing.T) {
		mock.ExpectExec("UPDATE site_stats").
			WithArgs(deltaVisits, deltaBytes, at, jobID, site).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))

		mock.ExpectExec("INSERT INTO site_stats").
			WithArgs(jobID, site, at, deltaVisits, deltaBytes, deltaVisits, int64(0), int64(0), int64(0)).
			WillReturnError(errors.New("db error"))

		err = ps.UpsertSiteStats(context.Background(), jobID, site, deltaVisits, deltaBytes, "2xx", at)
		assert.ErrorContains(t, err, "failed to insert site stats")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProgressStore_GetJob(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)
	jobID := uuid.New()
	startedAt := time.Now().UTC()
	status := store.RunRunning

	t.Run("success", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{"id", "job_id", "started_at", "finished_at", "status", "error_message"}).
			AddRow(jobID, jobID, startedAt, nil, status, nil)

		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs(jobID).
			WillReturnRows(rows)

		run, err := ps.GetJob(context.Background(), jobID)
		assert.NoError(t, err)
		assert.Equal(t, jobID, run.ID)
		assert.Equal(t, jobID, run.JobID)
		assert.Equal(t, startedAt, run.StartedAt)
		assert.Nil(t, run.FinishedAt)
		assert.Equal(t, status, run.Status)
		assert.Nil(t, run.ErrorMessage)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs(jobID).
			WillReturnError(pgx.ErrNoRows)

		_, err := ps.GetJob(context.Background(), jobID)
		assert.ErrorIs(t, err, store.ErrNotFound)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs(jobID).
			WillReturnError(errors.New("db error"))

		_, err := ps.GetJob(context.Background(), jobID)
		assert.ErrorContains(t, err, "failed to get job")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProgressStore_ListJobs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)
	jobID := uuid.New()
	startedAt := time.Now().UTC()
	status := store.RunRunning

	t.Run("success", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{"id", "job_id", "started_at", "finished_at", "status", "error_message"}).
			AddRow(jobID, jobID, startedAt, nil, status, nil)

		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs(&status, 10, 0).
			WillReturnRows(rows)

		runs, err := ps.ListJobs(context.Background(), &status, 10, 0)
		assert.NoError(t, err)
		assert.Len(t, runs, 1)
		assert.Equal(t, jobID, runs[0].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs((*store.JobRunStatus)(nil), 10, 0).
			WillReturnError(errors.New("db error"))

		_, err := ps.ListJobs(context.Background(), nil, 10, 0)
		assert.ErrorContains(t, err, "failed to list jobs")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan error", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{"id", "job_id", "started_at", "finished_at", "status", "error_message"}).
			AddRow("invalid-uuid", jobID, startedAt, nil, status, nil)

		mock.ExpectQuery("SELECT (.+) FROM job_runs").
			WithArgs((*store.JobRunStatus)(nil), 10, 0).
			WillReturnRows(rows)

		_, err := ps.ListJobs(context.Background(), nil, 10, 0)
		assert.ErrorContains(t, err, "failed to scan job row")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProgressStore_ListJobSites(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ps := NewProgressStoreWithPool(mock)
	jobID := uuid.New()
	site := "example.com"
	at := time.Now().UTC()
	var visits, bytes int64 = 10, 10240

	t.Run("success", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{
			"job_id", "site", "last_update", "visits", "bytes_total",
			"fetch_2xx", "fetch_3xx", "fetch_4xx", "fetch_5xx",
		}).
			AddRow(jobID, site, at, visits, bytes, int64(8), int64(1), int64(1), int64(0))

		mock.ExpectQuery("SELECT (.+) FROM site_stats").
			WithArgs(jobID, 10, 0).
			WillReturnRows(rows)

		stats, err := ps.ListJobSites(context.Background(), jobID, 10, 0)
		assert.NoError(t, err)
		assert.Len(t, stats, 1)
		assert.Equal(t, site, stats[0].Site)
		assert.Equal(t, visits, stats[0].Visits)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		mock.ExpectQuery("SELECT (.+) FROM site_stats").
			WithArgs(jobID, 10, 0).
			WillReturnError(errors.New("db error"))

		_, err := ps.ListJobSites(context.Background(), jobID, 10, 0)
		assert.ErrorContains(t, err, "failed to list job sites")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan error", func(t *testing.T) {
		rows := pgxmock.NewRows([]string{
			"job_id", "site", "last_update", "visits", "bytes_total",
			"fetch_2xx", "fetch_3xx", "fetch_4xx", "fetch_5xx",
		}).
			AddRow("invalid-uuid", site, at, visits, bytes, int64(8), int64(1), int64(1), int64(0))

		mock.ExpectQuery("SELECT (.+) FROM site_stats").
			WithArgs(jobID, 10, 0).
			WillReturnRows(rows)

		_, err := ps.ListJobSites(context.Background(), jobID, 10, 0)
		assert.ErrorContains(t, err, "failed to scan site stats row")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
