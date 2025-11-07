package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Postgres driver
	"go.uber.org/zap"
)

// PostgresProvider implements the database.Provider interface using PostgreSQL as the backend.
// It uses the `sqlx` library to simplify database interactions.
type PostgresProvider struct {
	DB *sqlx.DB
}

// NewPostgresProvider creates a new PostgreSQL connection and pings it to ensure it's alive.
// The dsn (Data Source Name) is expected in the standard format, e.g.,
// "postgres://user:pass@host:port/dbname?sslmode=disable"
func NewPostgresProvider(ctx context.Context, dsn string) (*PostgresProvider, error) {
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Ping the database to verify the connection is active.
	if err := db.PingContext(ctx); err != nil {
		// Close the connection if the ping fails, as it's unusable.
		err := db.Close()
		if err != nil {
			logging.L.Error("Failed to ping postgres", zap.Error(err))
		}
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	// TODO: Implement a database migration system (e.g., using golang-migrate/migrate)
	// to create and manage the database schema automatically.

	return &PostgresProvider{DB: db}, nil
}

// SaveCrawl inserts a new record into the 'crawls' table.
// It assumes a table schema like:
// CREATE TABLE crawls (
//
//	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
//	url TEXT NOT NULL,
//	fetched_at TIMESTAMPTZ NOT NULL,
//	blob_link TEXT NOT NULL,
//	blob_hash TEXT NOT NULL,
//	headers JSONB,
//	created_at TIMESTAMPTZ DEFAULT NOW()
//
// );
func (p *PostgresProvider) SaveCrawl(ctx context.Context, meta CrawlMetadata) (string, error) {
	// Marshal the headers map into a JSON byte slice for the JSONB column.
	headersBytes, err := json.Marshal(meta.Headers)
	if err != nil {
		return "", fmt.Errorf("failed to marshal headers to JSON: %w", err)
	}

	var crawlID string
	query := `
		INSERT INTO crawls (url, fetched_at, blob_link, blob_hash, headers)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	// Execute the query and scan the returned ID into the crawlID variable.
	err = p.DB.QueryRowxContext(ctx, query,
		meta.URL,
		meta.FetchedAt,
		meta.BlobLink,
		meta.BlobHash,
		headersBytes,
	).Scan(&crawlID)
	if err != nil {
		return "", fmt.Errorf("failed to execute insert statement: %w", err)
	}

	return crawlID, nil
}

// Close gracefully shuts down the database connection pool.
func (p *PostgresProvider) Close() error {
	if err := p.DB.Close(); err != nil {
		return fmt.Errorf("failed to close postgres connection: %w", err)
	}
	return nil
}
