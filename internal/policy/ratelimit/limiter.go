// Package ratelimit implements a token bucket rate limiter for per-domain concurrency and rate control.
package ratelimit

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
	"golang.org/x/time/rate"
)

// Limiter manages per-domain rate limits.
type Limiter struct {
	mu           sync.Mutex
	limiters     map[string]*rate.Limiter
	defaultRate  rate.Limit
	defaultBurst int
}

// Config holds rate limiter configuration.
type Config struct {
	DefaultRPS   float64
	DefaultBurst int
}

// New creates a new Limiter.
func New(cfg Config) *Limiter {
	r := rate.Limit(cfg.DefaultRPS)
	if cfg.DefaultRPS <= 0 {
		r = rate.Inf
	}
	burst := cfg.DefaultBurst
	if burst <= 0 {
		burst = 1
	}
	return &Limiter{
		limiters:     make(map[string]*rate.Limiter),
		defaultRate:  r,
		defaultBurst: burst,
	}
}

// Wait blocks until a token is available for the given domain, respecting the context.
func (l *Limiter) Wait(ctx context.Context, url string) error {
	domain := "unknown"
	if u, err := parseURL(url); err == nil {
		domain = u.Hostname()
	}
	l.mu.Lock()
	limiter, exists := l.limiters[domain]
	if !exists {
		limiter = rate.NewLimiter(l.defaultRate, l.defaultBurst)
		l.limiters[domain] = limiter
	}
	l.mu.Unlock()

	start := time.Now()
	err := limiter.Wait(ctx)
	// Only record delay if we actually waited (and didn't error immediately due to context)
	// But Wait returns error if context is canceled.
	// We should record the duration anyway if it was a rate limit wait.
	// If it returns error, we might still want to record?
	// Let's record if err is nil or if we waited significantly.
	// Actually, rate.Wait blocks.
	if err == nil {
		// We can't easily know how long we waited vs how long it took to get the lock/etc,
		// but measuring the whole Wait call is a good proxy for "delay introduced by rate limiter".
		// However, if the token was available immediately, duration is 0.
		duration := time.Since(start)
		if duration > time.Millisecond {
			telemetry.ObserveRateLimitDelay(domain, duration)
		}
	}
	if err != nil {
		return fmt.Errorf("rate limit wait: %w", err)
	}
	return nil
}

// AllowHeadless checks if headless fetch is allowed (placeholder for now).
func (l *Limiter) AllowHeadless(_ string, _ string, _ int) bool {
	return true
}

// AllowFetch checks if fetch is allowed (placeholder, usually true as we use WaitCtx).
func (l *Limiter) AllowFetch(_ string, _ string, _ int) bool {
	return true
}

// ReportResult provides feedback (placeholder).
func (l *Limiter) ReportResult(_ string, _ string, _ int, _ time.Duration) {
	// In a more advanced implementation, we could adjust rates based on 429s here.
}

func parseURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	return u, nil
}
