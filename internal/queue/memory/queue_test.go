package memory

import (
	"context"
	"testing"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestQueueEnqueueDequeue(t *testing.T) {
	t.Parallel()

	q := NewQueue(1)
	result := make(chan crawler.QueueItem, 1)
	errCh := make(chan error, 1)

	go func() {
		item, err := q.Dequeue(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		result <- item
	}()

	time.Sleep(10 * time.Millisecond) // allow goroutine to start
	job := crawler.QueueItem{JobID: "job-1"}
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("Dequeue() error = %v", err)
	case got := <-result:
		if got.JobID != "job-1" {
			t.Fatalf("expected job-1, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("dequeue did not return job")
	}
}

func TestQueueCancelationErrors(t *testing.T) {
	t.Parallel()

	qDequeue := NewQueue(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := qDequeue.Dequeue(ctx); err == nil ||
		err.Error() != "dequeue canceled: context canceled" {
		t.Fatalf("expected dequeue cancel error, got %v", err)
	}

	qEnqueue := NewQueue(1)
	if err := qEnqueue.Enqueue(context.Background(), crawler.QueueItem{JobID: "primed"}); err != nil {
		t.Fatalf("failed to prime enqueue queue: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := qEnqueue.Enqueue(ctx, crawler.QueueItem{}); err == nil ||
		err.Error() != "enqueue canceled: context canceled" {
		t.Fatalf("expected enqueue cancel error, got %v", err)
	}
}

func TestQueueClose(t *testing.T) {
	t.Parallel()

	q := NewQueue(1)
	q.Close()
	if _, err := q.Dequeue(context.Background()); err == nil || err.Error() != "queue closed" {
		t.Fatalf("expected queue closed error, got %v", err)
	}
	// Closing twice should be safe.
	q.Close()
}
