package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestWorker_ProcessJob_SuccessFlow(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-success",
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "abc123"}
	clock := &fakeClock{now: time.Unix(100, 0)}
	fetcher := &fakeFetcher{
		responses: map[string]crawler.FetchResponse{
			"https://example.com": {
				URL:        "https://example.com",
				StatusCode: http.StatusOK,
				Body:       []byte("<html>ok</html>"),
				Duration:   10 * time.Millisecond,
			},
		},
	}

	w := New(
		queue,
		jobStore,
		blobStore,
		publisher,
		hasher,
		clock,
		fetcher,
		nil,
		nil,
		nil,
		Config{
			ContentType: "text/html",
			BlobPrefix:  "pages",
			Topic:       "jobs",
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusSucceeded
	}, time.Second, 10*time.Millisecond)

	require.Len(t, jobStore.pages, 1)
	require.Equal(t, "pages/job-success/abc123.html", blobStore.lastPath)
	require.Len(t, publisher.messages, 1)
	require.Equal(t, crawler.JobCounters{PagesSucceeded: 1}, jobStore.lastCounters())
	cancel()
}

func TestWorker_ProcessJob_PublishFailureMarksJobFailed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-publish-fail",
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	publisher := newFakePublisher()
	publisher.err = errors.New("pub failure")
	hasher := &fakeHasher{hash: "deadbeef"}
	clock := &fakeClock{now: time.Unix(200, 0)}
	fetcher := &fakeFetcher{
		responses: map[string]crawler.FetchResponse{
			"https://example.com": {
				URL:        "https://example.com",
				StatusCode: http.StatusOK,
				Body:       []byte("<html>ok</html>"),
			},
		},
	}

	w := New(
		queue,
		jobStore,
		blobStore,
		publisher,
		hasher,
		clock,
		fetcher,
		nil,
		nil,
		nil,
		Config{
			ContentType: "text/html",
			BlobPrefix:  "pages",
			Topic:       "jobs",
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusFailed
	}, time.Second, 10*time.Millisecond)

	require.Equal(t, 1, jobStore.lastCounters().PagesFailed)
	require.Zero(t, len(publisher.messages))
	cancel()
}

func TestWorker_ProcessJob_HeadlessPromotionApplied(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-headless",
			Params: crawler.JobParameters{
				URLs:            []string{"https://example.com"},
				HeadlessAllowed: true,
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "beadfeed"}
	clock := &fakeClock{now: time.Unix(300, 0)}
	probeFetcher := &fakeFetcher{
		responses: map[string]crawler.FetchResponse{
			"https://example.com": {
				URL:        "https://example.com",
				StatusCode: http.StatusOK,
				Body:       []byte("<html>shell</html>"),
			},
		},
	}
	headlessFetcher := &fakeFetcher{
		responses: map[string]crawler.FetchResponse{
			"https://example.com": {
				URL:          "https://example.com/headless",
				StatusCode:   http.StatusOK,
				Body:         []byte("<html>rendered</html>"),
				Duration:     20 * time.Millisecond,
				UsedHeadless: true,
			},
		},
	}
	detector := &fakeDetector{promotions: map[string]bool{"https://example.com": true}}

	w := New(
		queue,
		jobStore,
		blobStore,
		publisher,
		hasher,
		clock,
		probeFetcher,
		headlessFetcher,
		detector,
		nil,
		Config{
			ContentType: "text/html",
			BlobPrefix:  "pages",
			Topic:       "jobs",
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusSucceeded
	}, time.Second, 10*time.Millisecond)

	require.Len(t, jobStore.pages, 1)
	require.True(t, jobStore.pages[0].UsedHeadless)
	require.Equal(t, "https://example.com/headless", jobStore.pages[0].URL)
	cancel()
}

// --- fakes ---

type fakeQueue struct {
	mu    sync.Mutex
	items []crawler.QueueItem
}

func (q *fakeQueue) Enqueue(_ context.Context, job crawler.QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, job)
	return nil
}

func (q *fakeQueue) Dequeue(ctx context.Context) (crawler.QueueItem, error) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			item := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return item, nil
		}
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return crawler.QueueItem{}, fmt.Errorf("queue dequeue context done: %w", ctx.Err())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

type fakeJobStore struct {
	mu       sync.Mutex
	statuses []statusUpdate
	pages    []crawler.PageRecord
}

type statusUpdate struct {
	status   crawler.JobStatus
	errText  string
	counters crawler.JobCounters
}

func newFakeJobStore() *fakeJobStore {
	return &fakeJobStore{}
}

func (f *fakeJobStore) CreateJob(context.Context, crawler.Job) error {
	return nil
}

func (f *fakeJobStore) UpdateJobStatus(
	_ context.Context,
	_ string,
	status crawler.JobStatus,
	errText string,
	counters crawler.JobCounters,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, statusUpdate{status: status, errText: errText, counters: counters})
	return nil
}

func (f *fakeJobStore) RecordPage(_ context.Context, page crawler.PageRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = append(f.pages, page)
	return nil
}

func TestWorkerBuildBlobPath(t *testing.T) {
	t.Parallel()

	w := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, Config{BlobPrefix: "/pages/"}, zap.NewNop())
	if got := w.buildBlobPath("job", "hash"); got != "pages/job/hash.html" {
		t.Fatalf("unexpected blob path: %s", got)
	}
	w.cfg.BlobPrefix = ""
	if got := w.buildBlobPath("job", "hash"); got != "job/hash.html" {
		t.Fatalf("unexpected fallback blob path: %s", got)
	}
}

func TestWorkerAllowHelpers(t *testing.T) {
	t.Parallel()

	w := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, Config{}, zap.NewNop())
	if !w.allowFetch("job", "url", 0) || !w.allowHeadless("job", "url", 0) {
		t.Fatal("expected allows with nil policy")
	}

	w.policy = fakePolicy{allowFetch: false, allowHeadless: true}
	if w.allowFetch("job", "url", 0) {
		t.Fatal("expected policy to block fetch")
	}
	if !w.allowHeadless("job", "url", 0) {
		t.Fatal("expected policy to allow headless")
	}
}

type fakePolicy struct {
	allowFetch    bool
	allowHeadless bool
}

func (f fakePolicy) AllowHeadless(string, string, int) bool {
	return f.allowHeadless
}

func (f fakePolicy) AllowFetch(string, string, int) bool {
	return f.allowFetch
}

func (f *fakeJobStore) GetJob(context.Context, string) (crawler.Job, error) {
	return crawler.Job{}, nil
}

func (f *fakeJobStore) ListPages(context.Context, string) ([]crawler.PageRecord, error) {
	return nil, nil
}

func (f *fakeJobStore) lastStatus() crawler.JobStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.statuses) == 0 {
		return ""
	}
	return f.statuses[len(f.statuses)-1].status
}

func (f *fakeJobStore) lastCounters() crawler.JobCounters {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.statuses) == 0 {
		return crawler.JobCounters{}
	}
	return f.statuses[len(f.statuses)-1].counters
}

type fakeBlobStore struct {
	mu       sync.Mutex
	objects  map[string][]byte
	lastPath string
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{objects: make(map[string][]byte)}
}

func (b *fakeBlobStore) PutObject(_ context.Context, path string, _ string, data []byte) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[path] = append([]byte(nil), data...)
	b.lastPath = path
	return "memory://" + path, nil
}

type fakePublisher struct {
	mu       sync.Mutex
	messages []map[string]any
	err      error
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{}
}

func (p *fakePublisher) Publish(_ context.Context, _ string, payload any) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if m, ok := payload.(map[string]any); ok {
		p.messages = append(p.messages, m)
	}
	return "msgid", nil
}

type fakeFetcher struct {
	mu        sync.Mutex
	responses map[string]crawler.FetchResponse
	errors    map[string]error
}

func (f *fakeFetcher) Fetch(_ context.Context, req crawler.FetchRequest) (crawler.FetchResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errors[req.URL]; ok {
		return crawler.FetchResponse{}, err
	}
	if resp, ok := f.responses[req.URL]; ok {
		return resp, nil
	}
	return crawler.FetchResponse{}, errors.New("not found")
}

type fakeDetector struct {
	promotions map[string]bool
}

func (d *fakeDetector) ShouldPromote(resp crawler.FetchResponse) bool {
	return d.promotions[resp.URL]
}

type fakeHasher struct {
	hash string
	err  error
}

func (h *fakeHasher) Hash(data []byte) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	if h.hash != "" {
		return h.hash, nil
	}
	return string(data), nil
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}
