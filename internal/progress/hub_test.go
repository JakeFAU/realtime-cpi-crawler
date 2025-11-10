package progress

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
)

// TestHubBatchBySize verifies the hub flushes immediately once the batch size limit is reached.
func TestHubBatchBySize(t *testing.T) {
	t.Parallel()

	sink := newStubSink()
	hub := NewHub(Config{
		BufferSize:     8,
		MaxBatchEvents: 2,
		MaxBatchWait:   time.Minute,
	}, sink)
	defer func() {
		require.NoError(t, hub.Close(context.Background()))
	}()

	evt := sampleEvent(StageJobStart)
	hub.Emit(evt)
	hub.Emit(evt)
	require.Eventually(t, func() bool {
		return len(sink.Batches()) == 1 && len(sink.Batches()[0]) == 2
	}, time.Second, 10*time.Millisecond)
}

// TestHubBatchByTimer verifies the timer-based flush kicks in when the batch is small.
func TestHubBatchByTimer(t *testing.T) {
	t.Parallel()

	sink := newStubSink()
	hub := NewHub(Config{
		BufferSize:     4,
		MaxBatchEvents: 10,
		MaxBatchWait:   25 * time.Millisecond,
	}, sink)
	defer func() {
		require.NoError(t, hub.Close(context.Background()))
	}()

	hub.Emit(sampleEvent(StageJobStart))
	require.Eventually(t, func() bool {
		return len(sink.Batches()) == 1
	}, time.Second, 5*time.Millisecond)
}

// TestHubEmitNonBlockingWithoutConsumers asserts Emit never blocks callers, even without sinks.
func TestHubEmitNonBlockingWithoutConsumers(t *testing.T) {
	t.Parallel()

	hub := &Hub{
		cfg:    Config{},
		events: make(chan Event),
		logger: zap.NewNop(),
	}
	start := time.Now()
	hub.Emit(sampleEvent(StageJobStart))
	require.Less(t, time.Since(start), 50*time.Millisecond)
}

// TestHubFlushOnClose ensures Close drains any buffered events before returning.
func TestHubFlushOnClose(t *testing.T) {
	t.Parallel()

	sink := newStubSink()
	hub := NewHub(Config{
		BufferSize:     4,
		MaxBatchEvents: 100,
		MaxBatchWait:   time.Minute,
	}, sink)

	evt := sampleEvent(StageJobStart)
	hub.Emit(evt)

	require.NoError(t, hub.Close(context.Background()))
	require.Len(t, sink.Batches(), 1)
	require.Len(t, sink.Batches()[0], 1)
}

type stubSink struct {
	mu      sync.Mutex
	batches [][]Event
}

func newStubSink() *stubSink {
	return &stubSink{batches: [][]Event{}}
}

func (s *stubSink) Consume(_ context.Context, batch []Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyBatch := append([]Event(nil), batch...)
	s.batches = append(s.batches, copyBatch)
	return nil
}

func (s *stubSink) Close(context.Context) error {
	return nil
}

func (s *stubSink) Batches() [][]Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]Event, len(s.batches))
	for i, b := range s.batches {
		out[i] = append([]Event(nil), b...)
	}
	return out
}

func sampleEvent(stage Stage) Event {
	id := uuid.New()
	return Event{
		JobID: UUIDToBytes(id),
		TS:    time.Now(),
		Stage: stage,
		Site:  "example.com",
		StatusClass: func() StatusClass {
			if stage == StageFetchDone {
				return Status2xx
			}
			return ""
		}(),
	}
}
