package collyfetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
)

const (
	robotsFallbackReasonTLSHandshake = "TLS handshake timeout"
)

var robotsRetryBackoff = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
}

type robotsAwareTransport struct {
	base  http.RoundTripper
	state *robotsProbeState
}

func (t *robotsAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("robots transport received nil request")
	}
	if t.state == nil || !isRobotsTxtRequest(req) {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("robots transport base roundtrip: %w", err)
		}
		return resp, nil
	}
	return t.state.roundTripWithRetry(req, t.base)
}

func isRobotsTxtRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return strings.EqualFold(req.URL.Path, "/robots.txt")
}

type robotsProbeState struct {
	status crawler.RobotsStatus
	reason string
}

func newRobotsProbeState() *robotsProbeState {
	return &robotsProbeState{}
}

func (s *robotsProbeState) apply(resp *crawler.FetchResponse) {
	if s == nil || resp == nil || s.status == crawler.RobotsStatusUnknown {
		return
	}
	resp.RobotsStatus = s.status
	resp.RobotsReason = s.reason
}

func (s *robotsProbeState) roundTripWithRetry(req *http.Request, base http.RoundTripper) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("nil request passed to roundTripWithRetry")
	}
	maxAttempts := len(robotsRetryBackoff) + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		cloneReq := cloneRequest(req)
		resp, err := base.RoundTrip(cloneReq)
		if err == nil {
			return resp, nil
		}
		if !isTransientTLSError(err) {
			return nil, fmt.Errorf("robots roundtrip non-transient: %w", err)
		}
		if attempt == maxAttempts-1 {
			s.markIndeterminate(robotsFallbackReasonTLSHandshake)
			return syntheticRobotsAllowAllResponse(req), nil
		}
		if err := sleepWithContext(req.Context(), robotsRetryBackoff[attempt]); err != nil {
			return nil, fmt.Errorf("robots roundtrip backoff sleep: %w", err)
		}
	}
	return nil, fmt.Errorf("robots roundtrip exhausted retries")
}

func (s *robotsProbeState) markIndeterminate(reason string) {
	if s.status == crawler.RobotsStatusIndeterminate {
		return
	}
	s.status = crawler.RobotsStatusIndeterminate
	s.reason = reason
	metrics.ObserveProbeTLSHandshakeTimeout()
}

func cloneRequest(req *http.Request) *http.Request {
	if req == nil {
		return nil
	}
	clone := req.Clone(req.Context())
	clone.Body = req.Body
	return clone
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("robots backoff sleep context: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func syntheticRobotsAllowAllResponse(req *http.Request) *http.Response {
	const body = "User-agent: *\nAllow: /"
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Header:        make(http.Header),
		Request:       req,
	}
}

func isTransientTLSError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "tls: handshake timeout")
}
