// Package simple contains placeholder policy implementations.
package simple

import (
	"context"
	"time"
)

// Policy is a permissive policy placeholder.
type Policy struct{}

// New creates a new Policy.
func New() *Policy {
	return &Policy{}
}

// AllowHeadless currently always returns true, placeholder for future heuristics.
func (Policy) AllowHeadless(_ string, _ string, _ int) bool {
	return true
}

// AllowFetch currently always returns true.
func (Policy) AllowFetch(_ string, _ string, _ int) bool {
	return true
}

// ReportResult is a no-op for the simple policy.
func (Policy) ReportResult(_ string, _ string, _ int, _ time.Duration) {}

// Wait returns immediately for the simple policy.
func (Policy) Wait(_ context.Context, _ string) error {
	return nil
}
