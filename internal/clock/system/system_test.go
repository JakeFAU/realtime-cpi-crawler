// Package system exercises the real-time clock adapter.
package system

import (
	"testing"
	"time"
)

// TestClockNowUTC ensures the clock returns UTC timestamps.
func TestClockNowUTC(t *testing.T) {
	t.Parallel()

	clk := New()
	requireNotNil(t, clk)

	before := time.Now().UTC().Add(-time.Second)
	got := clk.Now()
	after := time.Now().UTC().Add(time.Second)

	if got.Location() != time.UTC {
		t.Fatalf("expected UTC location, got %v", got.Location())
	}
	if got.Before(before) || got.After(after) {
		t.Fatalf("expected %v to be between %v and %v", got, before, after)
	}
}

// TestClockNowMonotonic checks successive timestamps are non-decreasing.
func TestClockNowMonotonic(t *testing.T) {
	t.Parallel()

	clk := New()
	first := clk.Now()
	second := clk.Now()
	if second.Before(first) {
		t.Fatalf("expected second call %v to be >= first %v", second, first)
	}
}

func requireNotNil(t *testing.T, v any) {
	t.Helper()
	if v == nil {
		t.Fatal("expected value to be non-nil")
	}
}
