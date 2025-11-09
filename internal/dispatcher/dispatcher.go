// Package dispatcher manages worker fan-out over the job queue.
package dispatcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
)

// Dispatcher fans out queue work to a pool of workers.
type Dispatcher struct {
	queue   crawler.Queue
	workers []*worker.Worker
}

// New creates a Dispatcher.
func New(queue crawler.Queue, workers []*worker.Worker) *Dispatcher {
	return &Dispatcher{
		queue:   queue,
		workers: workers,
	}
}

// Run starts all workers and blocks until the context finishes.
func (d *Dispatcher) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, w := range d.workers {
		wg.Add(1)
		go func(wk *worker.Worker) {
			defer wg.Done()
			wk.Run(ctx)
		}(w)
	}
	<-ctx.Done()
	wg.Wait()
}

// Enqueue proxies to the underlying queue.
func (d *Dispatcher) Enqueue(ctx context.Context, item crawler.QueueItem) error {
	if err := d.queue.Enqueue(ctx, item); err != nil {
		return fmt.Errorf("queue enqueue: %w", err)
	}
	return nil
}
