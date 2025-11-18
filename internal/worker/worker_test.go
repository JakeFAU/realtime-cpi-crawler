// Package worker contains integration-style tests for the worker pipeline.
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
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
)

// TestWorker_ProcessJob_SuccessFlow ensures a happy path job processes successfully.
func TestWorker_ProcessJob_SuccessFlow(t *testing.T) {
	t.Parallel()
	metrics.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const jobUUID = "11111111-1111-1111-1111-111111111111"
	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: jobUUID,
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	retrievalStore := newFakeRetrievalStore()
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "abc123"}
	clock := &fakeClock{now: time.Date(2025, time.January, 2, 15, 0, 0, 0, time.UTC)}
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
	idGen := &fakeIDGen{ids: []string{"page-success"}}
	emitter := &fakeProgressEmitter{}

	w := New(
		queue,
		jobStore,
		blobStore,
		retrievalStore,
		publisher,
		hasher,
		clock,
		fetcher,
		nil,
		nil,
		nil,
		idGen,
		emitter,
		Config{
			ContentType: "text/html",
			BlobPrefix:  "crawl",
			Topic:       "jobs",
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusSucceeded
	}, time.Second, 10*time.Millisecond)

	require.Len(t, jobStore.pages, 1)
	require.Equal(t, "page-success", jobStore.pages[0].ID)
	require.Len(t, blobStore.paths, 3)
	require.Equal(t, "crawl/202501/02/15/host=example.com/id=page-success/raw.html", blobStore.paths[0])
	require.Contains(t, blobStore.objects, "crawl/202501/02/15/host=example.com/id=page-success/meta.json")
	require.Len(t, publisher.messages, 1)
	msg := publisher.messages[0]
	require.Equal(t, "page-success", msg["crawl_id"])
	require.Equal(t, jobUUID, msg["job_id"])
	require.Equal(t, "example.com", msg["site"])
	require.Equal(t, "https://example.com", msg["url"])
	require.Equal(t, "memory://crawl/202501/02/15/host=example.com/id=page-success/raw.html", msg["html_blob"])
	require.Equal(t, "memory://crawl/202501/02/15/host=example.com/id=page-success/meta.json", msg["meta_blob"])
	require.Equal(t, pubSubSchemaVersion, msg["schema_version"])
	require.Equal(t, crawler.JobCounters{PagesSucceeded: 1}, jobStore.lastCounters())
	require.Len(t, retrievalStore.records(), 1)
	rec := retrievalStore.records()[0]
	require.Equal(t, clock.now, rec.JobStartedAt)
	require.Equal(t, 1, emitter.count(progress.StageJobStart))
	require.Equal(t, 1, emitter.count(progress.StageFetchDone))
	require.Equal(t, 1, emitter.count(progress.StageJobDone))
	cancel()
}

// TestWorker_ProcessJob_PublishFailureMarksJobFailed verifies publish errors mark jobs failed.
func TestWorker_ProcessJob_PublishFailureMarksJobFailed(t *testing.T) {
	t.Parallel()
	metrics.Init()

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

	idGen := &fakeIDGen{ids: []string{"page-fail"}}

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

// TestWorker_ProcessJob_HeadlessPromotionApplied confirms detector-triggered promotions.
func TestWorker_ProcessJob_HeadlessPromotionApplied(t *testing.T) {
	t.Parallel()
	metrics.Init()

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
	idGen := &fakeIDGen{ids: []string{"page-headless"}}

	w := New(
		queue,
		jobStore,
		blobStore,
		nil,
		publisher,
		hasher,
		clock,
		probeFetcher,
		headlessFetcher,
		detector,
		nil,
		idGen,
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

// TestWorker_ProcessJob_RetrievalStoreFailure ensures errors from the retrieval store fail the job.
func TestWorker_ProcessJob_RetrievalStoreFailure(t *testing.T) {
	t.Parallel()
	metrics.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue := &fakeQueue{
		items: []crawler.QueueItem{{
			JobID: "job-retrieval-fail",
			Params: crawler.JobParameters{
				URLs: []string{"https://example.com"},
			},
		}},
	}
	jobStore := newFakeJobStore()
	blobStore := newFakeBlobStore()
	retrievalStore := newFakeRetrievalStore()
	retrievalStore.err = errors.New("db down")
	publisher := newFakePublisher()
	hasher := &fakeHasher{hash: "deadbeef"}
	clock := &fakeClock{now: time.Unix(400, 0)}
	fetcher := &fakeFetcher{
		responses: map[string]crawler.FetchResponse{
			"https://example.com": {
				URL:        "https://example.com",
				StatusCode: http.StatusOK,
				Body:       []byte("<html>ok</html>"),
			},
		},
	}
	idGen := &fakeIDGen{ids: []string{"page-db-fail"}}

	w := New(
		queue,
		jobStore,
		blobStore,
		retrievalStore,
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
			ContentType: "text/html",
			BlobPrefix:  "pages",
		},
		zap.NewNop(),
	)

	go w.Run(ctx)

	require.Eventually(t, func() bool {
		return jobStore.lastStatus() == crawler.JobStatusFailed
	}, time.Second, 10*time.Millisecond)
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

type fakeProgressEmitter struct {
	mu     sync.Mutex
	events []progress.Event
}

func (f *fakeProgressEmitter) Emit(evt progress.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evt)
}

func (f *fakeProgressEmitter) count(stage progress.Stage) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, evt := range f.events {
		if evt.Stage == stage {
			total++
		}
	}
	return total
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

// TestWorkerBuildBlobPath checks blob path prefix handling.
func TestWorkerBuildBlobPath(t *testing.T) {
	t.Parallel()
	metrics.Init()

	ts := time.Date(2025, time.July, 4, 9, 0, 0, 0, time.UTC)
	w := New(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		Config{BlobPrefix: "/crawl/"},
		zap.NewNop(),
	)
	if got := w.buildBlobPath(ts, "Example.com", "page-1"); got != "crawl/202507/04/09/host=example.com/id=page-1" {
		t.Fatalf("unexpected blob path: %s", got)
	}
	w.cfg.BlobPrefix = ""
	if got := w.buildBlobPath(ts, "", "page-1"); got != "202507/04/09/host=unknown/id=page-1" {
		t.Fatalf("unexpected fallback blob path: %s", got)
	}
}

// TestWorkerAllowHelpers ensures policy hooks are honored.
func TestWorkerAllowHelpers(t *testing.T) {
	t.Parallel()
	metrics.Init()

	w := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, Config{}, zap.NewNop())
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
	paths    []string
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{objects: make(map[string][]byte)}
}

func (b *fakeBlobStore) PutObject(_ context.Context, path string, _ string, data []byte) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[path] = append([]byte(nil), data...)
	b.lastPath = path
	b.paths = append(b.paths, path)
	return "memory://" + path, nil
}

type fakeRetrievalStore struct {
	mu      sync.Mutex
	entries []crawler.RetrievalRecord
	err     error
}

func newFakeRetrievalStore() *fakeRetrievalStore {
	return &fakeRetrievalStore{}
}

func (f *fakeRetrievalStore) Close() error {
	return nil
}

func (f *fakeRetrievalStore) StoreRetrieval(_ context.Context, rec crawler.RetrievalRecord) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, rec)
	return nil
}

func (f *fakeRetrievalStore) records() []crawler.RetrievalRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]crawler.RetrievalRecord, len(f.entries))
	copy(out, f.entries)
	return out
}

type fakeIDGen struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeIDGen) NewID() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ids) == 0 {
		return "", fmt.Errorf("no ids configured")
	}
	id := f.ids[0]
	f.ids = f.ids[1:]
	return id, nil
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
