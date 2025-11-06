// Package storage defines the interfaces for a blob storage provider.
// This abstraction allows the application to be independent of a specific storage
// implementation (e.g., Google Cloud Storage, AWS S3, or the local filesystem).
package storage

import (
	"context"
)

// Provider defines the common interface for a blob storage provider.
// It abstracts the operation of saving data.
type Provider interface {
	// Save uploads data to a specified object path/key in the blob store.
	Save(ctx context.Context, objectName string, data []byte) error
}

// NoOpProvider is a mock storage provider that performs no operations.
// It is useful for testing or running the crawler in a dry-run mode where
// content is fetched but not saved.
type NoOpProvider struct{}

// Save for NoOpProvider does nothing and always returns nil.
func (n *NoOpProvider) Save(_ context.Context, _ string, _ []byte) error {
	return nil
}
