package simple

import "testing"

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
