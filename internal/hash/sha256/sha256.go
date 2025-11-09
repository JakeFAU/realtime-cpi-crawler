// Package sha256 provides SHA-256 hashing utilities.
package sha256

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hasher implements crawler.Hasher using SHA-256.
type Hasher struct{}

// New returns a SHA-256 hasher.
func New() *Hasher {
	return &Hasher{}
}

// Hash hashes the input and returns a hex digest.
func (h *Hasher) Hash(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
