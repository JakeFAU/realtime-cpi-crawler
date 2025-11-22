// Package memory stores blob content in-memory for development.
package memory

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// BlobStore stores artifacts in-memory and returns pseudo URIs.
type BlobStore struct {
	mu   sync.RWMutex
	data map[string][]byte
	uris map[string]string
}

// NewBlobStore creates a new in-memory blob store.
func NewBlobStore() *BlobStore {
	return &BlobStore{
		data: make(map[string][]byte),
		uris: make(map[string]string),
	}
}

// PutObject persists the content and returns a URI.
func (s *BlobStore) PutObject(_ context.Context, path string, _ string, data io.Reader) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byteData, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("failed to read data from reader: %w", err)
	}

	s.data[path] = append([]byte(nil), byteData...)
	uri := fmt.Sprintf("memory://%s", path)
	s.uris[path] = uri
	return uri, nil
}
