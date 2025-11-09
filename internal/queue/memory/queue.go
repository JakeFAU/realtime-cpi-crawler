// Package memory provides queue implementations for local development.
package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Queue is a bounded in-memory queue with context-aware operations.
type Queue struct {
	ch      chan crawler.QueueItem
	closeMu sync.Mutex
	closed  bool
}

// NewQueue constructs a new queue with the provided capacity.
func NewQueue(capacity int) *Queue {
	return &Queue{
		ch: make(chan crawler.QueueItem, capacity),
	}
}

// Enqueue pushes a job into the queue or returns if the context ends.
func (q *Queue) Enqueue(ctx context.Context, job crawler.QueueItem) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("enqueue canceled: %w", ctx.Err())
	case q.ch <- job:
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
