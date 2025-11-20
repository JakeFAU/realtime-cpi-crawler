package progress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Config controls buffering and batching for the Hub.
//   - BufferSize: size of the internal channel (default 4096).
//   - MaxBatchEvents: flush once this many events queue (default 1000).
//   - MaxBatchWait: flush after this duration even if the batch is small (default 500ms).
//   - SinkTimeout: per-sink timeout while flushing (default 10s).
//   - BaseContext: parent context passed to sink calls (defaults to context.Background()).
//   - Logger: optional structured logger used for warnings.
type Config struct {
	BufferSize     int
	MaxBatchEvents int
	MaxBatchWait   time.Duration
	SinkTimeout    time.Duration
	BaseContext    context.Context
	Logger         *zap.Logger
}

const (
	defaultBufferSize     = 4096
	defaultMaxBatchEvents = 1000
	defaultMaxBatchWait   = 500 * time.Millisecond
	defaultSinkTimeout    = 10 * time.Second
	dropLogInterval       = 5 * time.Second
)

// Hub aggregates Event streams and fans them out to registered sinks. It is
// safe for concurrent use by multiple goroutines and never blocks callers.
type Hub struct {
	cfg         Config
	sinks       []Sink
	events      chan Event
	stopCh      chan struct{}
	doneCh      chan struct{}
	logger      *zap.Logger
	dropLimiter rateLimiter
	dropped     atomic.Int64
	closed      atomic.Bool

	closeOnce sync.Once
	closeCtx  context.Context
}

// NewHub initializes a Hub and starts the background batching goroutine using
// the supplied sinks. The returned Hub is immediately ready to accept events.
func NewHub(cfg Config, sinks ...Sink) *Hub {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.MaxBatchEvents <= 0 {
		cfg.MaxBatchEvents = defaultMaxBatchEvents
	}
	if cfg.MaxBatchWait <= 0 {
		cfg.MaxBatchWait = defaultMaxBatchWait
	}
	if cfg.SinkTimeout <= 0 {
		cfg.SinkTimeout = defaultSinkTimeout
	}
	if cfg.BaseContext == nil {
		cfg.BaseContext = context.Background()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	h := &Hub{
		cfg:         cfg,
		sinks:       append([]Sink(nil), sinks...),
		events:      make(chan Event, cfg.BufferSize),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		logger:      logger,
		dropLimiter: rateLimiter{interval: dropLogInterval},
	}
	go h.run()
	return h
}

// Emit enqueues an Event for batching. It never blocks; if the buffer is full
// the event is dropped and a rate-limited warning is logged.
func (h *Hub) Emit(evt Event) {
	if h == nil {
		return
	}
	if h.closed.Load() {
		return
	}
	if err := evt.Validate(); err != nil {
		h.logger.Debug("discarding invalid progress event", zap.Error(err))
		return
	}
	select {
	case h.events <- evt:
	default:
		h.dropped.Add(1)
		if h.dropLimiter.Allow(time.Now()) {
			count := h.dropped.Swap(0)
			h.logger.Warn("progress events dropped due to backpressure", zap.Int64("dropped", count))
		}
	}
}

// Close drains remaining events, flushes sinks, and blocks until the background
// goroutine exits. It is safe to call multiple times; subsequent calls are
// ignored once shutdown begins.
func (h *Hub) Close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		h.closeCtx = ctx
		close(h.stopCh)
	})
	select {
	case <-h.doneCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("progress hub close wait: %w", ctx.Err())
	}
}

func (h *Hub) run() {
	defer close(h.doneCh)
	batch := make([]Event, 0, h.cfg.MaxBatchEvents)
	timer := time.NewTimer(h.cfg.MaxBatchWait)
	timer.Stop()
	timerActive := false
	for {
		select {
		case evt := <-h.events:
			batch = h.enqueueEvent(batch, evt, timer, &timerActive)
		case <-timer.C:
			timerActive = false
			if len(batch) > 0 {
				h.flush(batch)
				batch = batch[:0]
			}
		case <-h.stopCh:
			h.handleStop(batch, timer, &timerActive)
			return
		}
	}
}

func (h *Hub) enqueueEvent(batch []Event, evt Event, timer *time.Timer, timerActive *bool) []Event {
	batch = append(batch, evt)
	if len(batch) >= h.cfg.MaxBatchEvents {
		h.flush(batch)
		batch = batch[:0]
		h.stopTimer(timer, timerActive)
	} else if h.cfg.MaxBatchWait > 0 {
		h.resetTimer(timer, timerActive)
	}
	return batch
}

func (h *Hub) handleStop(batch []Event, timer *time.Timer, timerActive *bool) {
	h.stopTimer(timer, timerActive)
	for {
		select {
		case evt := <-h.events:
			batch = append(batch, evt)
			if len(batch) >= h.cfg.MaxBatchEvents {
				h.flush(batch)
				batch = batch[:0]
			}
		default:
			if len(batch) > 0 {
				h.flush(batch)
			}
			h.closeSinks()
			return
		}
	}
}

func (h *Hub) resetTimer(timer *time.Timer, timerActive *bool) {
	if h.cfg.MaxBatchWait <= 0 {
		return
	}
	if *timerActive {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	timer.Reset(h.cfg.MaxBatchWait)
	*timerActive = true
}

func (h *Hub) stopTimer(timer *time.Timer, timerActive *bool) {
	if !*timerActive {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	*timerActive = false
}

func (h *Hub) flush(batch []Event) {
	if len(batch) == 0 {
		return
	}
	copyBatch := append([]Event(nil), batch...)
	baseCtx := h.cfg.BaseContext
	for _, sink := range h.sinks {
		if sink == nil {
			continue
		}
		ctx := baseCtx
		cancel := func() {}
		if h.cfg.SinkTimeout > 0 {
			ctx, cancel = context.WithTimeout(baseCtx, h.cfg.SinkTimeout)
		}
		if err := sink.Consume(ctx, copyBatch); err != nil {
			h.logger.Warn("progress sink consume failed", zap.Error(err))
		}
		cancel()
	}
}

func (h *Hub) closeSinks() {
	ctx := h.closeCtx
	if ctx == nil {
		ctx = context.Background()
	}
	for _, sink := range h.sinks {
		if sink == nil {
			continue
		}
		if err := sink.Close(ctx); err != nil {
			h.logger.Warn("progress sink close failed", zap.Error(err))
		}
	}
}

type rateLimiter struct {
	interval time.Duration
	last     atomic.Int64
}

func (r *rateLimiter) Allow(now time.Time) bool {
	if r == nil || r.interval <= 0 {
		return true
	}
	nano := now.UnixNano()
	last := r.last.Load()
	if nano-last < r.interval.Nanoseconds() {
		return false
	}
	return r.last.CompareAndSwap(last, nano)
}
