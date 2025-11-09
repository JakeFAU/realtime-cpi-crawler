// Package memory includes tests for the in-memory blob store.
package memory

import (
	"context"
	"testing"
)

// TestBlobStorePutObjectCopiesData ensures stored blobs are immutable copies.
func TestBlobStorePutObjectCopiesData(t *testing.T) {
	t.Parallel()

	store := NewBlobStore()
	payload := []byte("content")
	uri, err := store.PutObject(context.Background(), "path/page.html", "text/html", payload)
	if err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
	if uri != "memory://path/page.html" {
		t.Fatalf("unexpected uri %s", uri)
	}
	payload[0] = 'C'
	stored := string(store.data["path/page.html"])
	if stored != "content" {
		t.Fatalf("expected stored copy to be immutable, got %q", stored)
	}
}
