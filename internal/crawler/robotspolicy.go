package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
	"go.uber.org/zap"
)

// RobotsEnforcer enforces robots.txt directives per host.
type RobotsEnforcer struct {
	client    *http.Client
	cache     sync.Map
	respect   bool
	userAgent string
	logger    *zap.Logger
}

// NewRobotsEnforcer builds a RobotsPolicy respecting the config toggle.
func NewRobotsEnforcer(respect bool, userAgent string, logger *zap.Logger) RobotsPolicy {
	if !respect {
		return &allowAllPolicy{}
	}
	return &RobotsEnforcer{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		respect:   true,
		userAgent: userAgent,
		logger:    logger,
	}
}

// Allowed implements RobotsPolicy.
func (r *RobotsEnforcer) Allowed(ctx context.Context, rawURL string) bool {
	if r == nil {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	data, err := r.load(ctx, parsed)
	if err != nil {
		r.logger.Warn("robots fetch failed; allowing access", zap.String("host", parsed.Host), zap.Error(err))
		return true
	}
	group := data.FindGroup(r.userAgent)
	if group == nil {
		return true
	}
	return group.Test(parsed.Path)
}

func (r *RobotsEnforcer) load(ctx context.Context, parsed *url.URL) (*robotstxt.RobotsData, error) {
	hostKey := strings.ToLower(parsed.Host)
	if data, ok := r.cache.Load(hostKey); ok {
		cached, assertOK := data.(*robotstxt.RobotsData)
		if !assertOK {
			return nil, fmt.Errorf("robots cache type mismatch: %T", data)
		}
		return cached, nil
	}

	robotsURL := *parsed
	robotsURL.Path = path.Join("/", "robots.txt")
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("new robots request: %w", err)
	}
	req.Header.Set("User-Agent", r.userAgent)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch robots: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			r.logger.Debug("Failed to close robots response body", zap.Error(cerr))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read robots body: %w", err)
	}
	data, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
	if err != nil {
		return nil, fmt.Errorf("parse robots: %w", err)
	}
	r.cache.Store(hostKey, data)
	return data, nil
}

type allowAllPolicy struct{}

func (a *allowAllPolicy) Allowed(context.Context, string) bool { return true }
