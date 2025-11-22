// Package gcs provides a BlobStore backed by Google Cloud Storage.
package gcs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
)

// Config captures the parameters required to connect to GCS.
type Config struct {
	Bucket string
}

// BlobStore writes artifacts to a configured GCS bucket.
type BlobStore struct {
	client *storage.Client
	bucket string
}

// New creates a GCS-backed blob store.
func New(client *storage.Client, cfg Config) (*BlobStore, error) {
	if client == nil {
		return nil, fmt.Errorf("storage client is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}
	return &BlobStore{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// PutObject uploads data to the configured bucket and returns a gs:// URI.
func (s *BlobStore) PutObject(ctx context.Context, path string, contentType string, r io.Reader) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	writer := s.client.Bucket(s.bucket).Object(path).NewWriter(ctx)
	if contentType != "" {
		writer.ContentType = contentType
	}
	if _, err := io.Copy(writer, r); err != nil {
		closeErr := writer.Close()
		if closeErr != nil {
			return "", fmt.Errorf("copy object: %w (close writer: %v)", err, closeErr)
		}
		return "", fmt.Errorf("copy object: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}
	return fmt.Sprintf("gs://%s/%s", s.bucket, path), nil
}
