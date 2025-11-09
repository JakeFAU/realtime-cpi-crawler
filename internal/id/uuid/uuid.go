// Package uuid provides ID generation helpers.
package uuid

import "github.com/google/uuid"

// Generator creates UUID v4 strings.
type Generator struct{}

// New creates a new Generator.
func New() *Generator {
	return &Generator{}
}

// NewID returns a UUID string.
func (Generator) NewID() (string, error) {
	return uuid.NewString(), nil
}
