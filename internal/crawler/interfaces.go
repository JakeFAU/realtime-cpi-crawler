// Package crawler declares the interfaces shared across the crawler subsystems.
package crawler

import (
	"context"
	"io"
	"time"
)

// JobStore persists job and page metadata.
type JobStore interface {
	CreateJob(ctx context.Context, job Job) error
	UpdateJobStatus(ctx context.Context, jobID string, status JobStatus, errText string, counters JobCounters) error
	RecordPage(ctx context.Context, page PageRecord) error
	GetJob(ctx context.Context, jobID string) (Job, error)
	ListPages(ctx context.Context, jobID string) ([]PageRecord, error)
}

// BlobStore writes raw artifacts and returns a URI.
type BlobStore interface {
	PutObject(ctx context.Context, path string, contentType string, data io.Reader) (string, error)
}

// RetrievalStore writes normalized retrieval rows to a downstream database.
type RetrievalStore interface {
	StoreRetrieval(ctx context.Context, record RetrievalRecord) error
	Close() error
}

// Publisher pushes completion events to Pub/Sub (or similar).
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) (string, error)
}

// Fetcher fetches a URL and returns the body plus metadata.
type Fetcher interface {
	Fetch(ctx context.Context, request FetchRequest) (FetchResponse, error)
}

// HeadlessDetector decides whether a headless fetch is warranted.
type HeadlessDetector interface {
	ShouldPromote(probe FetchResponse) bool
}

// Queue provides enqueue/dequeue semantics for crawl jobs.
type Queue interface {
	Enqueue(ctx context.Context, job QueueItem) error
	Dequeue(ctx context.Context) (QueueItem, error)
}

// Policy encapsulates admission control and rate limiting.
type Policy interface {
	AllowHeadless(jobID string, url string, depth int) bool
	AllowFetch(jobID string, url string, depth int) bool
	// ReportResult provides feedback to the policy (e.g. for rate limiter adjustments).
	ReportResult(jobID string, url string, statusCode int, duration time.Duration)
	// Wait blocks until the request is allowed to proceed.
	Wait(ctx context.Context, url string) error
}

// Hasher computes digests for deduplication/integrity.
type Hasher interface {
	Hash(data []byte) (string, error)
}

// Clock returns the current time (useful for testing).
type Clock interface {
	Now() time.Time
}

// IDGenerator produces job IDs (UUIDs).
type IDGenerator interface {
	NewID() (string, error)
}

// QueueItem wraps a job ready to run.
type QueueItem struct {
	JobID        string
	JobStartedAt time.Time
	Params       JobParameters
	Attempt      int
	Submitted    int64
}
