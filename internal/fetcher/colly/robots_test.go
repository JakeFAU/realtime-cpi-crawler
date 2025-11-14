package collyfetcher

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
)

func TestRobotsRetryReturnsAllowAllOnTimeout(t *testing.T) {
	t.Parallel()
	metrics.Init()

	state := newRobotsProbeState()
	base := &stubRoundTripper{
		results: []roundTripResult{
			{err: context.DeadlineExceeded},
			{err: context.DeadlineExceeded},
			{err: context.DeadlineExceeded},
			{err: context.DeadlineExceeded},
		},
	}
	transport := &robotsAwareTransport{
		base:  base,
		state: state,
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/robots.txt", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}
	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("resp close: %v", cerr)
		}
	})

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "User-agent: *\nAllow: /" {
		t.Fatalf("unexpected fallback body: %q", string(body))
	}
	if state.status != crawler.RobotsStatusIndeterminate {
		t.Fatalf("expected robots status to be indeterminate, got %q", state.status)
	}
	if state.reason != robotsFallbackReasonTLSHandshake {
		t.Fatalf("expected reason %q, got %q", robotsFallbackReasonTLSHandshake, state.reason)
	}
	if base.calls != 4 {
		t.Fatalf("expected 4 attempts, got %d", base.calls)
	}
}

func TestRobotsRetryStopsAfterSuccess(t *testing.T) {
	t.Parallel()
	metrics.Init()

	state := newRobotsProbeState()
	base := &stubRoundTripper{
		results: []roundTripResult{
			{err: context.DeadlineExceeded},
			{resp: httptest.NewRecorder().Result()},
		},
	}

	transport := &robotsAwareTransport{
		base:  base,
		state: state,
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/robots.txt", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("resp close: %v", cerr)
	}
	if base.calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", base.calls)
	}
	if state.status != crawler.RobotsStatusUnknown {
		t.Fatalf("expected robots status to remain unknown, got %q", state.status)
	}
}

type roundTripResult struct {
	resp *http.Response
	err  error
}

type stubRoundTripper struct {
	results []roundTripResult
	calls   int
}

func (s *stubRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	defer func() { s.calls++ }()
	if len(s.results) == 0 {
		return nil, context.DeadlineExceeded
	}
	idx := s.calls
	if idx >= len(s.results) {
		idx = len(s.results) - 1
	}
	res := s.results[idx]
	return res.resp, res.err
}
