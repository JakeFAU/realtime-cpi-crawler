package collyfetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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

// RobotsCacheTransport caches robots.txt responses to avoid redundant fetches.
type RobotsCacheTransport struct {
	base  http.RoundTripper
	cache *sync.Map // map[string]*http.Response (or []byte)
}

// NewRobotsCacheTransport creates a new RobotsCacheTransport.
func NewRobotsCacheTransport(base http.RoundTripper) *RobotsCacheTransport {
	return &RobotsCacheTransport{
		base:  base,
		cache: &sync.Map{},
	}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (t *RobotsCacheTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isRobotsTxtRequest(req) {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, fmt.Errorf("base roundtrip: %w", err)
		}
		return resp, nil
	}

	key := req.URL.String()
	if val, ok := t.cache.Load(key); ok {
		// Return cached response. We need to clone it because the body is read.
		// Actually, we need to store the body bytes and reconstruct the response.
		cached, ok := val.(*cachedResponse)
		if !ok {
			// Should not happen, but handle it
			t.cache.Delete(key)
		} else {
			return cached.toResponse(req), nil
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("base roundtrip: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		// Cache the response
		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close() //nolint:errcheck // We read it all, close it. Ignore error as we are replacing body.
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		// Reconstruct body for the caller
		resp.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

		cached := &cachedResponse{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       bodyBytes,
			Header:     resp.Header,
		}
		t.cache.Store(key, cached)
	}
	return resp, nil
}

type cachedResponse struct {
	StatusCode int
	Status     string
	Body       []byte
	Header     http.Header
}

func (c *cachedResponse) toResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode:    c.StatusCode,
		Status:        c.Status,
		Body:          io.NopCloser(strings.NewReader(string(c.Body))),
		ContentLength: int64(len(c.Body)),
		Header:        c.Header.Clone(),
		Request:       req,
	}
}
