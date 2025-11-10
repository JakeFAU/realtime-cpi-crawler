// Package progress defines the event structures emitted by the crawl workers.
package progress

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Stage denotes the type of milestone represented by an Event.
type Stage string

// Supported progress stages.
const (
	StageJobStart   Stage = "JOB_START"
	StageJobHB      Stage = "JOB_HEARTBEAT"
	StageJobDone    Stage = "JOB_DONE"
	StageJobError   Stage = "JOB_ERROR"
	StageFetchStart Stage = "FETCH_START"
	StageFetchDone  Stage = "FETCH_DONE"
)

// StatusClass is a coarse HTTP response grouping.
type StatusClass string

// Supported HTTP status classes tracked for fetch completions.
const (
	Status2xx   StatusClass = "2xx"
	Status3xx   StatusClass = "3xx"
	Status4xx   StatusClass = "4xx"
	Status5xx   StatusClass = "5xx"
	StatusOther StatusClass = "other"
)

// Event captures a single component of crawler progress.
type Event struct {
	// JobID uniquely identifies a job run using the 16-byte UUID form.
	JobID [16]byte
	// TS is the UTC timestamp recorded by the emitter.
	TS time.Time
	// Stage denotes which lifecycle or fetch milestone occurred.
	Stage Stage
	// Site optionally scopes fetch events to a host label.
	Site string
	// URL is the optional page URL; it should not contain credentials.
	URL string
	// Bytes carries the response size delta for the fetch.
	Bytes int64
	// Visits increments by one for each successful page completion.
	Visits int64
	// StatusClass groups HTTP response codes (2xx, 3xx, etc).
	StatusClass StatusClass
	// Dur captures execution latency for fetches and job completions.
	Dur time.Duration
	// Note lets emitters attach low-volume debug context (e.g. error text).
	Note string
}

// Validate performs coarse validation on Event payloads.
func (e Event) Validate() error {
	if e.JobID == [16]byte{} {
		return errors.New("job id is required")
	}
	if e.TS.IsZero() {
		return errors.New("timestamp is required")
	}
	switch e.Stage {
	case StageJobStart, StageJobHB, StageJobDone, StageJobError:
	case StageFetchStart:
		if e.Site == "" {
			return errors.New("fetch start requires site")
		}
	case StageFetchDone:
		if e.Site == "" {
			return errors.New("fetch done requires site")
		}
		if e.StatusClass == "" {
			return errors.New("fetch done requires status class")
		}
	default:
		return fmt.Errorf("unknown stage %q", e.Stage)
	}
	if e.Dur < 0 {
		return errors.New("duration must be >= 0")
	}
	return nil
}

// JobUUID converts the binary job ID to uuid.UUID for repositories.
func (e Event) JobUUID() uuid.UUID {
	return uuid.UUID(e.JobID)
}

// UUIDToBytes encodes a uuid.UUID into the Event form.
func UUIDToBytes(id uuid.UUID) [16]byte {
	var dest [16]byte
	copy(dest[:], id[:])
	return dest
}

// ClassifyStatus groups HTTP status codes for fetch events.
func ClassifyStatus(code int) StatusClass {
	switch {
	case code >= 200 && code < 300:
		return Status2xx
	case code >= 300 && code < 400:
		return Status3xx
	case code >= 400 && code < 500:
		return Status4xx
	case code >= 500 && code < 600:
		return Status5xx
	default:
		return StatusOther
	}
}
