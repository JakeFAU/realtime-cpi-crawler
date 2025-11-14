package postgres

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestStoreRetrievalInsertsRow(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	store, err := NewRetrievalStoreWithPool(mock, "retrievals")
	require.NoError(t, err)

	now := time.Unix(1700000000, 0).UTC()

	rec := crawler.RetrievalRecord{
		ID:          "uuid-v7",
		JobID:       "job-1",
		URL:         "https://example.com",
		Hash:        "abc123",
		BlobURI:     "gs://bucket/path",
		Headers:     http.Header{"Content-Type": {"text/html"}},
		StatusCode:  200,
		ContentType: "text/html",
		RetrievedAt: now,
		PartitionTS: now.Truncate(time.Hour),
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

	err = store.StoreRetrieval(context.Background(), rec)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
