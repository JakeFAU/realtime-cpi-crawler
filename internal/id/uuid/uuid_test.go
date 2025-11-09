// Package uuid includes tests for the UUID generator wrapper.
package uuid

import (
	"testing"

	goUUID "github.com/google/uuid"
)

// TestGeneratorNewID ensures generated IDs are unique and valid UUIDs.
func TestGeneratorNewID(t *testing.T) {
	t.Parallel()

	gen := New()
	id1, err := gen.NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	id2, err := gen.NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected unique IDs, got %s and %s", id1, id2)
	}
	if _, err := goUUID.Parse(id1); err != nil {
		t.Fatalf("id1 not valid UUID: %v", err)
	}
	if _, err := goUUID.Parse(id2); err != nil {
		t.Fatalf("id2 not valid UUID: %v", err)
	}
}
