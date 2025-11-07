package crawler

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"net"
	"time"
)

// ExponentialRetryPolicy implements RetryPolicy with jittered backoff.
type ExponentialRetryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// NewExponentialRetryPolicy builds a policy with sane defaults.
func NewExponentialRetryPolicy() *ExponentialRetryPolicy {
	return &ExponentialRetryPolicy{
		maxAttempts: 3,
		baseDelay:   250 * time.Millisecond,
		maxDelay:    5 * time.Second,
	}
}

// ShouldRetry decides whether the error is retryable.
func (p *ExponentialRetryPolicy) ShouldRetry(err error, attempt int) bool {
	if err == nil {
		return false
	}
	if attempt >= p.maxAttempts {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return true
}

// Backoff returns the wait duration before the next attempt.
func (p *ExponentialRetryPolicy) Backoff(attempt int) time.Duration {
	delay := float64(p.baseDelay) * math.Pow(2, float64(attempt))
	if delay > float64(p.maxDelay) {
		delay = float64(p.maxDelay)
	}
	jitter := p.randomJitter(time.Duration(delay) / 2)
	return time.Duration(delay/2) + jitter
}

func (p *ExponentialRetryPolicy) randomJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	bound := big.NewInt(int64(limit))
	n, err := rand.Int(rand.Reader, bound)
	if err != nil {
		return limit / 2
	}
	return time.Duration(n.Int64())
}
