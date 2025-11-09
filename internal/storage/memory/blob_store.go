// Package memory stores blob content in-memory for development.
package memory

import (
	"context"
	"fmt"
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
func (s *BlobStore) PutObject(_ context.Context, path string, _ string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[path] = append([]byte(nil), data...)
	uri := fmt.Sprintf("memory://%s", path)
	s.uris[path] = uri
	return uri, nil
}
