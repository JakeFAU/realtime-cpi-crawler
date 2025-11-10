package progress

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type exampleCountingSink struct {
	total int
}

func (s *exampleCountingSink) Consume(_ context.Context, batch []Event) error {
	s.total += len(batch)
	return nil
}

func (s *exampleCountingSink) Close(context.Context) error {
	return nil
}

// ExampleHub_Emit demonstrates emitting an event and flushing via Close.
func ExampleHub_Emit() {
	sink := &exampleCountingSink{}
	hub := NewHub(Config{
		BufferSize:     4,
		MaxBatchEvents: 1,
		MaxBatchWait:   time.Second,
	}, sink)

	hub.Emit(Event{
		JobID: UUIDToBytes(uuid.MustParse("00000000-0000-0000-0000-000000000001")),
		TS:    time.Unix(0, 0),
		Stage: StageJobStart,
	})
	if err := hub.Close(context.Background()); err != nil {
		panic(err)
	}

	fmt.Printf("events forwarded: %d\n", sink.total)
	// Output:
	// events forwarded: 1
}

// ExampleSink implements a custom Sink that totals response bytes.
func ExampleSink() {
	type bytesSink struct {
		bytes int64
	}
	var s bytesSink
	capture := sinkFunc(func(_ context.Context, batch []Event) error {
		for _, evt := range batch {
			s.bytes += evt.Bytes
		}
		return nil
	})
	hub := NewHub(Config{
		BufferSize:     2,
		MaxBatchEvents: 1,
		MaxBatchWait:   time.Second,
	}, capture)

	hub.Emit(Event{
		JobID:       UUIDToBytes(uuid.MustParse("00000000-0000-0000-0000-000000000002")),
		TS:          time.Unix(0, 0),
		Stage:       StageFetchDone,
		Site:        "example.com",
		StatusClass: Status2xx,
		Bytes:       512,
	})
	if err := hub.Close(context.Background()); err != nil {
		panic(err)
	}

	fmt.Printf("bytes downloaded: %d\n", s.bytes)
	// Output:
	// bytes downloaded: 512
}

type sinkFunc func(context.Context, []Event) error

func (f sinkFunc) Consume(ctx context.Context, batch []Event) error {
	return f(ctx, batch)
}

func (sinkFunc) Close(context.Context) error {
	return nil
}
