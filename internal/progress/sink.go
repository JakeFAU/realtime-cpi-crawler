package progress

import "context"

// Sink consumes batches of progress events. Implementations must be safe for
// repeated calls, honor ctx deadlines, and may be invoked concurrently.
type Sink interface {
	Consume(ctx context.Context, batch []Event) error
	Close(ctx context.Context) error
}

// Emitter publishes individual events; Hub satisfies this interface so workers
// can remain agnostic about how events are buffered or persisted.
type Emitter interface {
	Emit(evt Event)
}
