package postgres

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

func TestStoreRetrievalInsertsRow(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	retrievalStore, err := NewRetrievalStoreWithPool(mock, "retrievals", "job_runs")
	require.NoError(t, err)

	now := time.Unix(1700000000, 0).UTC()

	rec := crawler.RetrievalRecord{
		ID:           "uuid-v7",
		JobID:        "019a83aa-158e-70e8-92a2-99fc386d75ef",
		JobStartedAt: now,
		URL:          "https://example.com",
		Hash:         "abc123",
		BlobURI:      "gs://bucket/path",
		Headers:      http.Header{"Content-Type": {"text/html"}},
		StatusCode:   200,
		ContentType:  "text/html",
		RetrievedAt:  now,
		PartitionTS:  now.Truncate(time.Hour),
	}

	mock.ExpectExec("INSERT INTO job_runs").
		WithArgs(rec.JobID, rec.JobID, rec.JobStartedAt, store.RunRunning).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectExec("INSERT INTO retrievals").
		WithArgs(
			rec.ID,
			rec.JobID,
			rec.JobStartedAt,
			rec.PartitionTS,
			rec.RetrievedAt,
			rec.URL,
			rec.Hash,
			rec.BlobURI,
			[]byte(`{"Content-Type":["text/html"]}`),
			rec.StatusCode,
			rec.ContentType,
			rec.ParentID,
			rec.ParentTimestamp,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = retrievalStore.StoreRetrieval(context.Background(), rec)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRetrievalSkipsJobRunEnsureWithoutProgressTable(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	retrievalStore, err := NewRetrievalStoreWithPool(mock, "retrievals", "")
	require.NoError(t, err)

	now := time.Unix(1700000000, 0).UTC()
	rec := crawler.RetrievalRecord{
		ID:           "uuid-v7",
		JobID:        "019a83aa-158e-70e8-92a2-99fc386d75ef",
		JobStartedAt: now,
		URL:          "https://example.com",
		Hash:         "abc123",
		BlobURI:      "gs://bucket/path",
		Headers:      http.Header{"Content-Type": {"text/html"}},
		StatusCode:   200,
		ContentType:  "text/html",
		RetrievedAt:  now,
		PartitionTS:  now.Truncate(time.Hour),
	}

	mock.ExpectExec("INSERT INTO retrievals").
		WithArgs(
			rec.ID,
			rec.JobID,
			rec.JobStartedAt,
			rec.PartitionTS,
			rec.RetrievedAt,
			rec.URL,
			rec.Hash,
			rec.BlobURI,
			[]byte(`{"Content-Type":["text/html"]}`),
			rec.StatusCode,
			rec.ContentType,
			rec.ParentID,
			rec.ParentTimestamp,
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = retrievalStore.StoreRetrieval(context.Background(), rec)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
