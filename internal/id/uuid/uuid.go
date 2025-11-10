// Package uuid provides ID generation helpers.
package uuid

import (
	"fmt"

	"github.com/google/uuid"
)

// Generator creates UUID v7 strings.
type Generator struct{}

// NewUUIDGenerator creates a new Generator.
func NewUUIDGenerator() *Generator {
	return &Generator{}
}

// NewID returns a UUID7 string.
func (Generator) NewID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate uuid7: %w", err)
	}
	return id.String(), nil
}

// NewRawID returns a UUID7.
func (Generator) NewRawID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate uuid7: %w", err)
	}
	return id, nil
}

// NewV4ID returns a UUIDv4 string (mainly for compatibility purposes).
func (Generator) NewV4ID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate uuid4: %w", err)
	}
	return id.String(), nil
}

// NewRawV4ID returns a UUIDv4 (mainly for compatibility purposes).
func (Generator) NewRawV4ID() (uuid.UUID, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate uuid4: %w", err)
	}
	return id, nil
}
