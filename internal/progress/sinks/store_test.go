package sinks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

// TestStoreSinkPersistsEvents ensures visits/bytes are collapsed per site before persisting.
func TestStoreSinkPersistsEvents(t *testing.T) {
	t.Parallel()

	repo := &fakeProgressRepo{}
	sink := NewStoreSink(repo, nil)
	jobUUID := uuid.New()
	jobID := progress.UUIDToBytes(jobUUID)
	now := time.Now()

	batch := []progress.Event{
		{JobID: jobID, Stage: progress.StageJobStart, TS: now},
		{
			JobID:       jobID,
			Stage:       progress.StageFetchDone,
			Site:        "example.com",
			Bytes:       100,
			Visits:      1,
			StatusClass: progress.Status2xx,
			TS:          now.Add(1 * time.Second),
		},
		{
			JobID:       jobID,
			Stage:       progress.StageFetchDone,
			Site:        "example.com",
			Bytes:       50,
			Visits:      2,
			StatusClass: progress.Status2xx,
			TS:          now.Add(2 * time.Second),
		},
		{JobID: jobID, Stage: progress.StageJobDone, TS: now.Add(3 * time.Second), Dur: 3 * time.Second},
	}

	require.NoError(t, sink.Consume(context.Background(), batch))

	require.Len(t, repo.starts, 1)
	require.Len(t, repo.completes, 1)
	require.Len(t, repo.siteStats, 1)
	stats := repo.siteStats[0]
	require.Equal(t, int64(3), stats.deltaVisits)
	require.Equal(t, int64(150), stats.deltaBytes)
}

// TestStoreSinkHandlesErrors surfaces repository failures back to the caller.
func TestStoreSinkHandlesErrors(t *testing.T) {
	t.Parallel()

	repo := &fakeProgressRepo{fail: true}
	sink := NewStoreSink(repo, nil)
	jobID := progress.UUIDToBytes(uuid.New())
	err := sink.Consume(context.Background(), []progress.Event{
		{JobID: jobID, Stage: progress.StageJobStart, TS: time.Now()},
	})
	require.Error(t, err)
}

type fakeProgressRepo struct {
	fail      bool
	starts    []uuid.UUID
	completes []uuid.UUID
	siteStats []siteCall
}

type siteCall struct {
	jobID       uuid.UUID
	site        string
	deltaVisits int64
	deltaBytes  int64
	statusClass string
}

func (f *fakeProgressRepo) UpsertJobStart(_ context.Context, jobID uuid.UUID, startedAt time.Time) error {
	if f.fail {
		return assertErr("start")
	}
	_ = startedAt
	f.starts = append(f.starts, jobID)
	return nil
}

func (f *fakeProgressRepo) CompleteJob(
	_ context.Context,
	jobID uuid.UUID,
	finishedAt time.Time,
	status store.JobRunStatus,
	errMsg *string,
) error {
	if f.fail {
		return assertErr("complete")
	}
	_ = finishedAt
	_ = status
	_ = errMsg
	f.completes = append(f.completes, jobID)
	return nil
}

func (f *fakeProgressRepo) UpsertSiteStats(
	_ context.Context,
	jobID uuid.UUID,
	site string,
	deltaVisits int64,
	deltaBytes int64,
	statusClass string,
	at time.Time,
) error {
	if f.fail {
		return assertErr("site")
	}
	_ = at
	f.siteStats = append(f.siteStats, siteCall{
		jobID:       jobID,
		site:        site,
		deltaVisits: deltaVisits,
		deltaBytes:  deltaBytes,
		statusClass: statusClass,
	})
	return nil
}

func (f *fakeProgressRepo) GetJob(context.Context, uuid.UUID) (store.JobRun, error) {
	return store.JobRun{}, assertErr("read")
}

func (f *fakeProgressRepo) ListJobs(context.Context, *store.JobRunStatus, int, int) ([]store.JobRun, error) {
	return nil, assertErr("list")
}

func (f *fakeProgressRepo) ListJobSites(context.Context, uuid.UUID, int, int) ([]store.SiteStats, error) {
	return nil, assertErr("sites")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
