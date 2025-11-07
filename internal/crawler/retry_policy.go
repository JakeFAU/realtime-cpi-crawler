package crawler

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

// ExponentialRetryPolicy implements RetryPolicy with jittered backoff.
type ExponentialRetryPolicy struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	mu          sync.Mutex
	rnd         *rand.Rand
}

// NewExponentialRetryPolicy builds a policy with sane defaults.
func NewExponentialRetryPolicy() *ExponentialRetryPolicy {
	return &ExponentialRetryPolicy{
		maxAttempts: 3,
		baseDelay:   250 * time.Millisecond,
		maxDelay:    5 * time.Second,
		rnd:         rand.New(rand.NewSource(time.Now().UnixNano())),
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
	if errors.As(err, &netErr) && netErr.Temporary() {
		return true
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

func (p *ExponentialRetryPolicy) randomJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Duration(p.rnd.Int63n(int64(max)))
}
