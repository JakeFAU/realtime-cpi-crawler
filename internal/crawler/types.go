// Package crawler defines core types shared across subsystems.
package crawler

import (
	"net/http"
	"time"
)

// JobStatus represents the lifecycle state of a crawl job.
type JobStatus string

// Job status values persisted in the job store.
const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCanceled  JobStatus = "canceled"
)

// JobParameters captures per-job configuration knobs requested by the client.
type JobParameters struct {
	URLs                  []string          `json:"urls"`
	MaxDepth              int               `json:"max_depth"`
	MaxPages              int               `json:"max_pages"`
	BudgetSeconds         int               `json:"budget_seconds"`
	HeadlessAllowed       bool              `json:"headless_allowed" mapstructure:"headless_allowed"`
	HeadlessProvided      bool              `json:"-" mapstructure:"headless_provided"`
	RespectRobots         bool              `json:"respect_robots" mapstructure:"respect_robots"`
	RespectRobotsProvided bool              `json:"-" mapstructure:"respect_robots_provided"`
	PerDomainCaps         map[string]int    `json:"per_domain_caps"`
	Tags                  map[string]string `json:"tags"`
	AllowDomains          []string          `json:"allow_domains"`
	DenyDomains           []string          `json:"deny_domains"`
}

// Job represents the metadata persisted for each submitted crawl request.
type Job struct {
	ID         string        `json:"id"`
	Status     JobStatus     `json:"status"`
	Submitted  time.Time     `json:"submitted_at"`
	Started    *time.Time    `json:"started_at,omitempty"`
	Finished   *time.Time    `json:"finished_at,omitempty"`
	ErrorText  string        `json:"error_text,omitempty"`
	Parameters JobParameters `json:"parameters"`
	Counters   JobCounters   `json:"counters"`
}

// JobCounters tracks success/failure stats per job.
type JobCounters struct {
	PagesSucceeded int `json:"pages_succeeded"`
	PagesFailed    int `json:"pages_failed"`
	Retries        int `json:"retries"`
}

// PageRecord is persisted for each fetched page.
type PageRecord struct {
	ID           string         `json:"id,omitempty"`
	JobID        string         `json:"job_id"`
	JobStartedAt time.Time      `json:"job_started_at"`
	URL          string         `json:"url"`
	StatusCode   int            `json:"status_code"`
	UsedHeadless bool           `json:"used_headless"`
	FetchedAt    time.Time      `json:"fetched_at"`
	DurationMs   int64          `json:"duration_ms"`
	ContentHash  string         `json:"content_hash"`
	Headers      http.Header    `json:"headers"`
	BlobURI      string         `json:"blob_uri"`
	Metrics      map[string]int `json:"metrics,omitempty"`
}

// RetrievalRecord captures details persisted to downstream storage.
type RetrievalRecord struct {
	ID              string
	JobID           string
	JobStartedAt    time.Time
	URL             string
	Hash            string
	BlobURI         string
	Headers         http.Header
	StatusCode      int
	ContentType     string
	SizeBytes       int
	RetrievedAt     time.Time
	PartitionTS     time.Time
	ParentID        *string
	ParentTimestamp *time.Time
}

// FetchRequest captures everything needed to fetch a URL.
type FetchRequest struct {
	JobID                 string
	JobStartedAt          time.Time
	URL                   string
	Depth                 int
	UseHeadless           bool
	Headers               http.Header
	RespectRobots         bool
	RespectRobotsProvided bool
}

// FetchResponse is the result returned by a Fetcher implementation.
type FetchResponse struct {
	URL          string
	StatusCode   int
	Headers      http.Header
	Body         []byte
	Duration     time.Duration
	UsedHeadless bool
	Screenshot   []byte
	JobID        string
	JobStartedAt time.Time
	RobotsStatus RobotsStatus
	RobotsReason string
}

// JobResult is returned by the API result endpoint.
type JobResult struct {
	Job   Job
	Pages []PageRecord
}

// RobotsStatus captures the outcome of evaluating robots.txt.
type RobotsStatus string

const (
	// RobotsStatusUnknown is the zero/default value.
	RobotsStatusUnknown RobotsStatus = ""
	// RobotsStatusIndeterminate indicates the fetcher could not definitively read robots.txt.
	RobotsStatusIndeterminate RobotsStatus = "indeterminate"
)
