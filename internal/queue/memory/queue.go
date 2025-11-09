// Package memory provides queue implementations for local development.
package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
)

// Queue is a bounded in-memory queue with context-aware operations.
type Queue struct {
	ch      chan crawler.QueueItem
	closeMu sync.Mutex
	closed  bool
	meters  *metrics.Collectors
}

// NewQueue constructs a new queue with the provided capacity.
func NewQueue(capacity int) *Queue {
	return &Queue{
		ch: make(chan crawler.QueueItem, capacity),
	}
}

// WithMetrics attaches a metrics collector and returns the queue for chaining.
func (q *Queue) WithMetrics(m *metrics.Collectors) *Queue {
	q.meters = m
	return q
}

// Enqueue pushes a job into the queue or returns if the context ends.
func (q *Queue) Enqueue(ctx context.Context, job crawler.QueueItem) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("enqueue canceled: %w", ctx.Err())
	case q.ch <- job:
		q.observe("enqueue")
		return nil
	}
}

// Dequeue pops the next job, respecting context cancellation.
func (q *Queue) Dequeue(ctx context.Context) (crawler.QueueItem, error) {
	select {
	case <-ctx.Done():
		return crawler.QueueItem{}, fmt.Errorf("dequeue canceled: %w", ctx.Err())
	case job, ok := <-q.ch:
		if !ok {
			return crawler.QueueItem{}, errors.New("queue closed")
		}
		q.observe("dequeue")
		return job, nil
	}
}

// Close closes the underlying channel for shutdown.
func (q *Queue) Close() {
	q.closeMu.Lock()
	defer q.closeMu.Unlock()
	if q.closed {
		return
	}
	close(q.ch)
	q.closed = true
}

func (q *Queue) observe(operation string) {
	if q.meters == nil {
		return
	}
	q.meters.RecordQueueEvent(operation, len(q.ch))
}
