package sinks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
)

// TestPrometheusSinkRecordsMetrics ensures counters and histograms are incremented from events.
func TestPrometheusSinkRecordsMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	sink, err := NewPrometheusSink(reg)
	require.NoError(t, err)

	jobID := progress.UUIDToBytes(uuid.New())
	batch := []progress.Event{
		{JobID: jobID, TS: time.Now(), Stage: progress.StageJobStart},
		{
			JobID:       jobID,
			TS:          time.Now().Add(10 * time.Second),
			Stage:       progress.StageFetchDone,
			Site:        "example.com",
			Bytes:       1024,
			Visits:      1,
			StatusClass: progress.Status2xx,
			Dur:         200 * time.Millisecond,
		},
		{JobID: jobID, TS: time.Now().Add(15 * time.Second), Stage: progress.StageJobDone, Dur: 15 * time.Second},
	}

	require.NoError(t, sink.Consume(context.Background(), batch))

	require.Equal(t, 1.0, testutil.ToFloat64(sink.jobsStarted))
	require.Equal(t, 1.0, testutil.ToFloat64(sink.jobsCompleted.WithLabelValues("success")))
	require.Equal(t, 0.0, testutil.ToFloat64(sink.jobsCompleted.WithLabelValues("error")))
	require.Equal(t, 0.0, testutil.ToFloat64(sink.jobsRunning))

	require.InDelta(
		t,
		1.0,
		testutil.ToFloat64(sink.fetchRequests.WithLabelValues("example.com", string(progress.Status2xx))),
		1e-9,
	)
	require.InDelta(t, 1024.0, testutil.ToFloat64(sink.fetchBytes.WithLabelValues("example.com")), 1e-9)
	require.Equal(t, 1, testutil.CollectAndCount(sink.fetchDuration, "crawler_fetch_duration_seconds"))
}
