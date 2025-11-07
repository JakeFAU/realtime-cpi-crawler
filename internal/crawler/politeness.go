package crawler

import (
	"context"
	"strings"
	"sync"
	"time"
)

// visitTracker provides thread-safe visited URL tracking to prevent revisits.
type visitTracker interface {
	MarkIfNew(url string) bool
}

type concurrentVisitTracker struct {
	seen sync.Map
}

func newConcurrentVisitTracker() *concurrentVisitTracker {
	return &concurrentVisitTracker{}
}

// MarkIfNew stores the URL if it has not been seen before and returns true.
func (t *concurrentVisitTracker) MarkIfNew(url string) bool {
	if url == "" {
		return false
	}
	_, loaded := t.seen.LoadOrStore(url, struct{}{})
	return !loaded
}

// domainBlocker tracks repeated forbidden responses and blocks hosts on excess.
type domainBlocker interface {
	IsBlocked(host string) bool
	MarkForbidden(host string) bool
}

type thresholdDomainBlocker struct {
	mu        sync.Mutex
	threshold int
	counts    map[string]int
	blocked   map[string]struct{}
}

func newThresholdDomainBlocker(threshold int) *thresholdDomainBlocker {
	if threshold <= 0 {
		threshold = defaultForbiddenAttempts
	}
	return &thresholdDomainBlocker{
		threshold: threshold,
		counts:    make(map[string]int),
		blocked:   make(map[string]struct{}),
	}
}

func (b *thresholdDomainBlocker) IsBlocked(host string) bool {
	if host == "" {
		return false
	}
	key := strings.ToLower(host)
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.blocked[key]
	return ok
}

// MarkForbidden increments the counter for host and returns true once blocked.
func (b *thresholdDomainBlocker) MarkForbidden(host string) bool {
	if host == "" {
		return false
	}
	key := strings.ToLower(host)
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, blocked := b.blocked[key]; blocked {
		return true
	}
	b.counts[key]++
	if b.counts[key] >= b.threshold {
		b.blocked[key] = struct{}{}
		return true
	}
	return false
}

// pauseController abstracts how the crawler backs off when throttled.
type pauseController interface {
	Pause(ctx context.Context, delay time.Duration)
}

type timerPauseController struct{}

func (p *timerPauseController) Pause(ctx context.Context, delay time.Duration) {
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
