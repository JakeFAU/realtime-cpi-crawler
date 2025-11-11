package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSanitizeSite(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard http", "http://example.com/path", "example.com"},
		{"standard https", "https://Example.com/path", "example.com"},
		{"no scheme", "example.com/path", "example.com"},
		{"just host", "example.com", "example.com"},
		{"host with port", "example.com:8080", "example.com"},
		{"ip address", "192.168.1.1", "192.168.1.1"},
		{"invalid url", "http://%", "unknown"},
		{"empty string", "", "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeSite(tc.input); got != tc.expected {
				t.Errorf("SanitizeSite(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestInit(t *testing.T) {
	// Reset collectors for testing purposes.
	crawlerPagesTotal = nil
	crawlerBytesTotal = nil
	httpRequestsTotal = nil
	httpRequestDurationSeconds = nil

	// Call Init multiple times to test idempotency.
	Init()
	Init()

	if crawlerPagesTotal == nil || crawlerBytesTotal == nil ||
		httpRequestsTotal == nil || httpRequestDurationSeconds == nil {
		t.Fatal("Init() did not initialize metrics collectors")
	}

	// A simple check to see if a metric can be used.
	crawlerPagesTotal.WithLabelValues("test.com", "success").Inc()
	if val := testutil.ToFloat64(crawlerPagesTotal); val != 1 {
		t.Errorf("Expected crawlerPagesTotal to be 1, got %f", val)
	}
}

// Fuzz test for SanitizeSite.
func FuzzSanitizeSite(f *testing.F) {
	testcases := []string{"http://example.com", "https://google.com", "ftp://example.com"}
	for _, tc := range testcases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, orig string) {
		sanitized := SanitizeSite(orig)
		if sanitized == "" {
			t.Errorf("SanitizeSite(%q) returned an empty string", orig)
		}
	})
}
