package storage

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"go.uber.org/zap"
)

// GCSProvider implements the storage.Provider interface for Google Cloud Storage.
type GCSProvider struct {
	Client     *storage.Client
	BucketName string
}

// NewGCSProvider initializes a new GCS client and verifies the connection.
// Authentication is handled automatically via Google's "Application Default Credentials" (ADC).
func NewGCSProvider(ctx context.Context, bucketName string) (*GCSProvider, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	// Check if the bucket exists and we have permissions to access it.
	// This is a good practice to fail fast on startup if configuration is wrong.
	bkt := client.Bucket(bucketName)
	if _, err := bkt.Attrs(ctx); err != nil {
		err := client.Close()
		if err != nil {
			logging.L.Warn("Failed to close GCS client after bucket existence check failure", zap.Error(err))
		}
		return nil, fmt.Errorf("failed to get GCS bucket '%s' attributes: %w", bucketName, err)
	}

	return &GCSProvider{
		Client:     client,
		BucketName: bucketName,
	}, nil
}

// Save uploads the given data to a specific object in the GCS bucket.
func (g *GCSProvider) Save(ctx context.Context, objectName string, data []byte) error {
	// Get a writer for the GCS object.
	wc := g.Client.Bucket(g.BucketName).Object(objectName).NewWriter(ctx)

	// Write the data to the object.
	if _, err := wc.Write(data); err != nil {
		// Even if the write fails, we should still try to close the writer to clean up resources.
		errTwo := wc.Close() // We ignore the error from this Close, as the primary error is the write failure.
		logging.L.Warn("Failed to close GCS writer after write failure", zap.Error(err), zap.Error(errTwo))
		return fmt.Errorf("failed to write data to GCS object %s: %w", objectName, err)
	}

	// Close must be called to finalize the upload. It flushes any buffered data.
	if err := wc.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer for object %s: %w", objectName, err)
	}

	return nil
}
