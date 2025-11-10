package sinks

import (
	"context"

	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
)

// LogSink emits structured logs for debugging progress streams. It is useful
// during development or audits where a durable store is unavailable.
type LogSink struct {
	logger *zap.Logger
}

// NewLogSink wires a Zap logger to the sink interface.
func NewLogSink(logger *zap.Logger) *LogSink {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LogSink{logger: logger}
}

// Consume logs each event in the batch using structured fields.
func (s *LogSink) Consume(_ context.Context, batch []progress.Event) error {
	for _, evt := range batch {
		fields := []zap.Field{
			zap.ByteString("job_id", evt.JobID[:]),
			zap.String("stage", string(evt.Stage)),
			zap.String("site", evt.Site),
			zap.String("url", evt.URL),
			zap.Int64("bytes", evt.Bytes),
			zap.Int64("visits", evt.Visits),
			zap.String("status_class", string(evt.StatusClass)),
			zap.Duration("dur", evt.Dur),
			zap.String("note", evt.Note),
		}
		s.logger.Info("progress event", fields...)
	}
	return nil
}

// Close implements the Sink interface; it performs no action.
func (s *LogSink) Close(context.Context) error {
	return nil
}
