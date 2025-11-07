// Package database_test contains unit tests for the database package.
package database_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresProvider_SaveCrawl(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close() //nolint:errcheck

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	p := database.PostgresProvider{DB: sqlxDB}

	meta := database.CrawlMetadata{
		URL:       "http://example.com",
		FetchedAt: time.Now(),
		BlobLink:  "pages/2023-10-27/somehash.html",
		BlobHash:  "somehash",
		Headers:   map[string]any{"key": "value"},
	}

	expectedCrawlID := "test-crawl-id"
	// The query is a regex because the actual query is multiline
	query := `INSERT INTO crawls (url, fetched_at, blob_link, blob_hash, headers) VALUES ($1, $2, $3, $4, $5) RETURNING id`

	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(meta.URL, meta.FetchedAt, meta.BlobLink, meta.BlobHash, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(expectedCrawlID))

	crawlID, err := p.SaveCrawl(context.Background(), meta)
	require.NoError(t, err)
	assert.Equal(t, expectedCrawlID, crawlID)

	// Verify that all expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err, "sqlmock expectations not met")
}
