// Package database_test contains unit tests for the database package.
package database_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresProvider_SaveCrawl_JsonMarshalError(t *testing.T) {
	p := database.PostgresProvider{}
	meta := database.CrawlMetadata{
		Headers: map[string]any{
			"key": make(chan int), // a channel can't be marshaled to JSON
		},
	}

	_, err := p.SaveCrawl(context.Background(), meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal headers to JSON")
}

func TestPostgresProvider_SaveCrawl_QueryError(t *testing.T) {
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

	queryErr := errors.New("query failed")
	query := `INSERT INTO crawls (url, fetched_at, blob_link, blob_hash, headers) VALUES ($1, $2, $3, $4, $5) RETURNING id`

	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(meta.URL, meta.FetchedAt, meta.BlobLink, meta.BlobHash, sqlmock.AnyArg()).
		WillReturnError(queryErr)

	_, err = p.SaveCrawl(context.Background(), meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute insert statement")

	err = mock.ExpectationsWereMet()
	assert.NoError(t, err, "sqlmock expectations not met")
}

func TestPostgresProvider_Close_Error(t *testing.T) {
	mockDB, mock, err := sqlmock.New()

	require.NoError(t, err)

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	p := database.PostgresProvider{DB: sqlxDB}

	closeErr := errors.New("close failed")

	mock.ExpectClose().WillReturnError(closeErr)

	err = p.Close()

	require.Error(t, err)

	assert.Contains(t, err.Error(), "failed to close postgres connection")

	err = mock.ExpectationsWereMet()

	assert.NoError(t, err, "sqlmock expectations not met")
}
