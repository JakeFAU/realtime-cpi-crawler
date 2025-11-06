package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Postgres driver
)

// PostgresProvider implements the database.Provider interface using PostgreSQL as the backend.
// It uses the `sqlx` library to simplify database interactions.
type PostgresProvider struct {
	db *sqlx.DB
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
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	// TODO: Implement a database migration system (e.g., using golang-migrate/migrate)
	// to create and manage the database schema automatically.

	return &PostgresProvider{db: db}, nil
}

// SaveCrawl inserts a new record into the 'crawls' table.
// It assumes a table schema like:
// CREATE TABLE crawls (
//
//	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
//	url TEXT NOT NULL,
//	storage_key TEXT NOT NULL,
//	fetched_at TIMESTAMPTZ NOT NULL,
//	metadata JSONB,
//	created_at TIMESTAMPTZ DEFAULT NOW()
//
// );
func (p *PostgresProvider) SaveCrawl(ctx context.Context, meta CrawlMetadata) (string, error) {
	// Marshal the metadata map into a JSON byte slice for the JSONB column.
	metadataBytes, err := json.Marshal(meta.Metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata to JSON: %w", err)
	}

	var crawlID string
	query := `
		INSERT INTO crawls (url, storage_key, fetched_at, metadata)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	// Execute the query and scan the returned ID into the crawlID variable.
	err = p.db.QueryRowxContext(ctx, query,
		meta.URL,
		meta.StorageKey,
		meta.FetchedAt,
		metadataBytes,
	).Scan(&crawlID)
	if err != nil {
		return "", fmt.Errorf("failed to execute insert statement: %w", err)
	}

	return crawlID, nil
}

// Close gracefully shuts down the database connection pool.
func (p *PostgresProvider) Close() error {
	if err := p.db.Close(); err != nil {
		return fmt.Errorf("failed to close postgres connection: %w", err)
	}
	return nil
}
