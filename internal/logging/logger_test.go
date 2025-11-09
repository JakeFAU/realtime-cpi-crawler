// Package logging includes tests for the zap logger helpers.
package logging

import "testing"

// TestNewDevelopmentLogger confirms the development logger builds and logs.
func TestNewDevelopmentLogger(t *testing.T) {
	t.Parallel()

	logger, err := New(true)
	if err != nil {
		t.Fatalf("New(true) error = %v", err)
	}
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}
	defer logger.Sync() //nolint:errcheck // best-effort flush
	logger.Info("development logger ready")
}

// TestNewProductionLogger ensures the production logger configuration succeeds.
func TestNewProductionLogger(t *testing.T) {
	t.Parallel()

	logger, err := New(false)
	if err != nil {
		t.Fatalf("New(false) error = %v", err)
	}
	if logger == nil {
		t.Fatal("expected logger to be non-nil")
	}
	defer logger.Sync() //nolint:errcheck // best-effort flush
	logger.Info("production logger ready")
}
