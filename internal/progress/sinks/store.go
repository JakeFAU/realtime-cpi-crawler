package sinks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
)

// StoreSink persists progress deltas via a store.ProgressRepository. It batches
// site-level counters to reduce write amplification.
type StoreSink struct {
	repo   store.ProgressRepository
	logger *zap.Logger
}

// NewStoreSink constructs a StoreSink for the provided repository.
func NewStoreSink(repo store.ProgressRepository, logger *zap.Logger) *StoreSink {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StoreSink{repo: repo, logger: logger}
}

// Consume collapses site deltas and forwards them to the repository. It respects
// ctx deadlines and returns any repository errors verbatim.
func (s *StoreSink) Consume(ctx context.Context, batch []progress.Event) error {
	if s == nil || s.repo == nil {
		return nil
	}
	stats := make(map[statsKey]*statsDelta)

	for _, evt := range batch {
		jobID := evt.JobUUID()
		switch evt.Stage {
		case progress.StageJobStart, progress.StageJobDone, progress.StageJobError:
			if err := s.handleJobEvent(ctx, jobID, evt); err != nil {
				return err
			}
		case progress.StageFetchDone:
			s.recordSiteStats(stats, jobID, evt)
		}
	}

	for key, delta := range stats {
		if delta.visits == 0 && delta.bytes == 0 {
			continue
		}
		if err := s.repo.UpsertSiteStats(
			ctx,
			key.jobID,
			key.site,
			delta.visits,
			delta.bytes,
			key.statusClass,
			delta.at,
		); err != nil {
			return fmt.Errorf("upsert site stats: %w", err)
		}
	}
	return nil
}

func (s *StoreSink) handleJobEvent(ctx context.Context, jobID uuid.UUID, evt progress.Event) error {
	switch evt.Stage {
	case progress.StageJobStart:
		if err := s.repo.UpsertJobStart(ctx, jobID, evt.TS); err != nil {
			return fmt.Errorf("upsert job start: %w", err)
		}
	case progress.StageJobDone:
		if err := s.repo.CompleteJob(ctx, jobID, evt.TS, store.RunSuccess, nil); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}
	case progress.StageJobError:
		var note *string
		if evt.Note != "" {
			note = &evt.Note
		}
		if err := s.repo.CompleteJob(ctx, jobID, evt.TS, store.RunError, note); err != nil {
			return fmt.Errorf("complete job: %w", err)
		}
	}
	return nil
}

func (s *StoreSink) recordSiteStats(stats map[statsKey]*statsDelta, jobID uuid.UUID, evt progress.Event) {
	if evt.Site == "" {
		return
	}
	key := statsKey{
		jobID:       jobID,
		site:        evt.Site,
		statusClass: string(evt.StatusClass),
	}
	stat := stats[key]
	if stat == nil {
		stat = &statsDelta{}
		stats[key] = stat
	}
	stat.visits += evt.Visits
	stat.bytes += evt.Bytes
	if evt.TS.After(stat.at) || stat.at.IsZero() {
		stat.at = evt.TS
	}
}

// Close implements the Sink interface; it performs no action.
func (s *StoreSink) Close(context.Context) error {
	return nil
}

type statsKey struct {
	jobID       uuid.UUID
	site        string
	statusClass string
}

type statsDelta struct {
	visits int64
	bytes  int64
	at     time.Time
}
