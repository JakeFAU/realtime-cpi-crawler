package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// JobStore provides an in-memory implementation for development/testing.
type JobStore struct {
	mu    sync.RWMutex
	jobs  map[string]crawler.Job
	pages map[string][]crawler.PageRecord
}

// NewJobStore constructs a JobStore.
func NewJobStore() *JobStore {
	return &JobStore{
		jobs:  make(map[string]crawler.Job),
		pages: make(map[string][]crawler.PageRecord),
	}
}

// CreateJob stores a new job in queued status.
func (s *JobStore) CreateJob(_ context.Context, job crawler.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return errors.New("job already exists")
	}
	s.jobs[job.ID] = job
	return nil
}

// UpdateJobStatus updates the status and counters for a job.
func (s *JobStore) UpdateJobStatus(
	_ context.Context,
	jobID string,
	status crawler.JobStatus,
	errText string,
	counters crawler.JobCounters,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return errors.New("job not found")
	}
	job.Status = status
	job.ErrorText = errText
	job.Counters = counters
	now := time.Now().UTC()
	if status == crawler.JobStatusRunning && job.Started == nil {
		job.Started = pointerTime(now)
	}
	if isTerminal(status) {
		job.Finished = pointerTime(now)
	}
	s.jobs[jobID] = job
	return nil
}

// RecordPage appends a page row for a job.
func (s *JobStore) RecordPage(_ context.Context, page crawler.PageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pages[page.JobID] = append(s.pages[page.JobID], page)
	return nil
}

// GetJob fetches a job by ID.
func (s *JobStore) GetJob(_ context.Context, jobID string) (crawler.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return crawler.Job{}, errors.New("job not found")
	}
	return job, nil
}

// ListPages returns all recorded pages for a job.
func (s *JobStore) ListPages(_ context.Context, jobID string) ([]crawler.PageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pages := s.pages[jobID]
	out := make([]crawler.PageRecord, len(pages))
	copy(out, pages)
	return out, nil
}

func pointerTime(t time.Time) *time.Time {
	ts := t
	return &ts
}

func isTerminal(status crawler.JobStatus) bool {
	switch status {
	case crawler.JobStatusSucceeded, crawler.JobStatusFailed, crawler.JobStatusCanceled:
		return true
	default:
		return false
	}
}
