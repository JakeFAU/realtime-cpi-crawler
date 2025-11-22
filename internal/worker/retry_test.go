package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type countingFetcher struct {
	mu       sync.Mutex
	attempts int
	fails    int
}

func (f *countingFetcher) Fetch(_ context.Context, req crawler.FetchRequest) (crawler.FetchResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts <= f.fails {
		return crawler.FetchResponse{}, errors.New("transient error")
	}
	return crawler.FetchResponse{
		StatusCode: 200,
		Body:       []byte("success"),
		URL:        req.URL,
	}, nil
}

func TestWorker_RetryLogic(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	if _, _, err := telemetry.InitTelemetry(context.Background(), &cfg); err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-retry",
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "abc123retry"}
	clock := &fakeClock{now: time.Now()}

	// Fails 2 times, succeeds on 3rd attempt
	fetcher := &countingFetcher{fails: 2}

	idGen := &fakeIDGen{ids: []string{"page-retry"}}

	w := New(
		queue,
		jobStore,
		blobStore,
		nil,
		publisher,
		hasher,
		clock,
		fetcher,
		nil,
		nil,
		nil,
		idGen,
		nil,
		Config{
			ContentType:      "text/html",
			BlobPrefix:       "retry",
			MaxRetries:       3,
			RetryBackoffBase: 1 * time.Millisecond,
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusSucceeded
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, 3, fetcher.attempts)
	require.Equal(t, 1, jobStore.lastCounters().PagesSucceeded)
	cancel()
}

func TestWorker_RetryExhausted(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	if _, _, err := telemetry.InitTelemetry(context.Background(), &cfg); err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-retry-fail",
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "abc123fail"}
	clock := &fakeClock{now: time.Now()}

	// Fails 5 times, max retries is 3
	fetcher := &countingFetcher{fails: 5}

	idGen := &fakeIDGen{ids: []string{"page-retry-fail"}}

	w := New(
		queue,
		jobStore,
		blobStore,
		nil,
		publisher,
		hasher,
		clock,
		fetcher,
		nil,
		nil,
		nil,
		idGen,
		nil,
		Config{
			ContentType:      "text/html",
			BlobPrefix:       "retry",
			MaxRetries:       3,
			RetryBackoffBase: 1 * time.Millisecond,
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusFailed
	}, 2*time.Second, 10*time.Millisecond)

	// Initial attempt + 3 retries = 4 attempts
	require.Equal(t, 4, fetcher.attempts)
	require.Equal(t, 1, jobStore.lastCounters().PagesFailed)
	cancel()
}
