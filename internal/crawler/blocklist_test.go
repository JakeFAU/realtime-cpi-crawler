package crawler

import "testing"

func TestDomainPatternBlocklist(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		bl := newDomainPatternBlocklist([]string{"example.org"})
		if bl == nil {
			t.Fatalf("expected blocklist to be created")
		}
		if !bl.IsBlocked("example.org") {
			t.Fatalf("expected example.org to be blocked")
		}
		if bl.IsBlocked("sub.example.org") {
			t.Fatalf("did not expect subdomains to match exact entry")
		}
	})

	t.Run("wildcard suffix", func(t *testing.T) {
		bl := newDomainPatternBlocklist([]string{"*.ru"})
		if bl == nil {
			t.Fatalf("expected blocklist to be created")
		}
		cases := []struct {
			host    string
			blocked bool
		}{
			{"example.ru", true},
			{"sub.domain.ru", true},
			{"ru", true},
			{"example.com", false},
		}
		for _, tc := range cases {
			if got := bl.IsBlocked(tc.host); got != tc.blocked {
				t.Fatalf("host %q blocked=%v, want %v", tc.host, got, tc.blocked)
			}
		}
	})

	t.Run("nil blocklist", func(t *testing.T) {
		var bl *domainPatternBlocklist
		if bl.IsBlocked("anything") {
			t.Fatalf("nil blocklist should never block")
		}
	})
}
