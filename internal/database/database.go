// Package database defines the interfaces for persisting crawl metadata.
// By using an interface, we decouple the application from a specific database implementation,
// allowing for easier testing and flexibility in the future.
package database

import (
	"context"
	"time"
)

// CrawlMetadata holds the essential information about a completed crawl job.
// This struct is what gets stored in the database.
type CrawlMetadata struct {
	URL       string         `db:"url"`
	FetchedAt time.Time      `db:"fetched_at"`
	BlobLink  string         `db:"blob_link"`
	BlobHash  string         `db:"blob_hash"`
	Headers   map[string]any `db:"headers"`
}

// Provider defines the common interface for our database layer.
// This allows us to use a real Postgres database in production and a mock (NoOpProvider)
// in tests or for local development.	type Provider interface {
type Provider interface {
	// SaveCrawl saves the metadata of a completed crawl to the database.
	// It returns a unique ID for the saved record or an error if the operation fails.
	SaveCrawl(ctx context.Context, meta CrawlMetadata) (string, error)

	// Close terminates the database connection and releases any resources.
	Close() error
}

// NoOpProvider is a mock database provider that performs no operations.
// It is useful for testing or running the crawler without a real database connection.
type NoOpProvider struct{}

// SaveCrawl for NoOpProvider does nothing and returns a dummy ID and no error.
func (n *NoOpProvider) SaveCrawl(_ context.Context, _ CrawlMetadata) (string, error) {
	return "noop-crawl-id", nil
}

// Close for NoOpProvider does nothing and returns no error.
func (n *NoOpProvider) Close() error { return nil }
