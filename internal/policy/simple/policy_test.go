// Package simple includes tests for the permissive policy implementation.
package simple

import "testing"

// TestPolicyAllowMethods ensures the placeholder policy allows operations.
func TestPolicyAllowMethods(t *testing.T) {
	t.Parallel()

	p := New()
	if !p.AllowHeadless("job", "https://example.com", 0) {
		t.Fatal("expected AllowHeadless to return true")
	}
	if !p.AllowFetch("job", "https://example.com", 0) {
		t.Fatal("expected AllowFetch to return true")
	}
}
