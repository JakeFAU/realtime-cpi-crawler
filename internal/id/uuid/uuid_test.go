// Package uuid includes tests for the UUID generator wrapper.
package uuid

import (
	"testing"

	"github.com/google/uuid"
)

// TestGeneratorNewID ensures generated IDs are unique and valid UUIDs.
func TestGeneratorNewID(t *testing.T) {
	t.Parallel()

	gen := NewUUIDGenerator()
	id1, err := gen.NewRawID()
	if err != nil {
		t.Fatalf("NewRawID() error = %v", err)
	}
	id2, err := gen.NewRawID()
	if err != nil {
		t.Fatalf("NewRawID() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected unique IDs, got %s and %s", id1, id2)
	}
}

func TestGeneratorNewV4ID(t *testing.T) {
	t.Parallel()

	gen := NewUUIDGenerator()
	id1, err := gen.NewRawV4ID()
	if err != nil {
		t.Fatalf("NewRawID() error = %v", err)
	}
	id2, err := gen.NewRawV4ID()
	if err != nil {
		t.Fatalf("NewRawV4ID() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected unique IDs, got %s and %s", id1, id2)
	}
}

func TestGeneratorNewStringID(t *testing.T) {
	t.Parallel()

	gen := NewUUIDGenerator()
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
	// Validate UUID format
	_, err = uuid.Parse(id1)
	if err != nil {
		t.Fatalf("NewID() returned invalid UUID: %s", id1)
	}
	_, err = uuid.Parse(id2)
	if err != nil {
		t.Fatalf("NewID() returned invalid UUID: %s", id2)
	}
}

func TestGeneratorNewStringV4ID(t *testing.T) {
	t.Parallel()

	gen := NewUUIDGenerator()
	id1, err := gen.NewV4ID()
	if err != nil {
		t.Fatalf("NewV4ID() error = %v", err)
	}
	id2, err := gen.NewV4ID()
	if err != nil {
		t.Fatalf("NewV4ID() error = %v", err)
	}
	if id1 == id2 {
		t.Fatalf("expected unique IDs, got %s and %s", id1, id2)
	}
	// Validate UUID format
	_, err = uuid.Parse(id1)
	if err != nil {
		t.Fatalf("NewV4ID() returned invalid UUID: %s", id1)
	}
	_, err = uuid.Parse(id2)
	if err != nil {
		t.Fatalf("NewV4ID() returned invalid UUID: %s", id2)
	}
}
