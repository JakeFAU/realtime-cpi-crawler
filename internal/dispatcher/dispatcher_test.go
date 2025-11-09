// Package dispatcher contains tests for worker coordination.
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
)

// TestDispatcherRunStartsWorkers ensures workers begin processing and stop on cancel.
func TestDispatcherRunStartsWorkers(t *testing.T) {
	t.Parallel()

	queue := &blockingQueue{started: make(chan struct{}, 1)}
	w := worker.New(
		queue,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		worker.Config{},
		zap.NewNop(),
	)
	dispatch := New(queue, []*worker.Worker{w})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		dispatch.Run(ctx)
		close(done)
	}()

	select {
	case <-queue.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not begin dequeuing")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop after context cancel")
	}
}

// TestDispatcherEnqueueForwardsErrors verifies queue errors are wrapped for callers.
func TestDispatcherEnqueueForwardsErrors(t *testing.T) {
	t.Parallel()

	queue := &errorQueue{err: errors.New("boom")}
	dispatch := New(queue, nil)

	err := dispatch.Enqueue(context.Background(), crawler.QueueItem{JobID: "job"})
	if err == nil || err.Error() != "queue enqueue: boom" {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

type blockingQueue struct {
	started chan struct{}
}

func (q *blockingQueue) Enqueue(_ context.Context, _ crawler.QueueItem) error {
	select {
	case q.started <- struct{}{}:
	default:
	}
	return nil
}

func (q *blockingQueue) Dequeue(ctx context.Context) (crawler.QueueItem, error) {
	select {
	case q.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return crawler.QueueItem{}, fmt.Errorf("blocking dequeue canceled: %w", ctx.Err())
}

type errorQueue struct {
	err error
}

func (q *errorQueue) Enqueue(context.Context, crawler.QueueItem) error {
	return q.err
}

func (q *errorQueue) Dequeue(context.Context) (crawler.QueueItem, error) {
	return crawler.QueueItem{}, nil
}
