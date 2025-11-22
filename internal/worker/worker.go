// Package worker implements the crawl pipeline execution loop.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	goUUID "github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/id/uuid"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Config controls Worker behavior.
type Config struct {
	ContentType string
	BlobPrefix  string
	Topic       string
	JobTimeout  time.Duration
	// RequestTimeout bounds a single fetch (probe or headless). Headless fetches
	// may still stop earlier via their own navigation timeout, but will never
	// exceed this cap.
	// RequestTimeout bounds a single fetch (probe or headless). Headless fetches
	// may still stop earlier via their own navigation timeout, but will never
	// exceed this cap.
	RequestTimeout   time.Duration
	MaxRetries       int
	RetryBackoffBase time.Duration
}

// Worker consumes queue items and executes the fetch pipeline.
type Worker struct {
	queue           crawler.Queue
	jobStore        crawler.JobStore
	blobStore       crawler.BlobStore
	retrievalStore  crawler.RetrievalStore
	publisher       crawler.Publisher
	hasher          crawler.Hasher
	clock           crawler.Clock
	probeFetcher    crawler.Fetcher
	headlessFetcher crawler.Fetcher
	detector        crawler.HeadlessDetector
	policy          crawler.Policy
	idGen           crawler.IDGenerator
	cfg             Config
	logger          *zap.Logger
	progress        progress.Emitter
}

const pubSubSchemaVersion = "1"

// New constructs a Worker.
func New(
	queue crawler.Queue,
	jobStore crawler.JobStore,
	blobStore crawler.BlobStore,
	retrievalStore crawler.RetrievalStore,
	publisher crawler.Publisher,
	hasher crawler.Hasher,
	clock crawler.Clock,
	probe crawler.Fetcher,
	headless crawler.Fetcher,
	detector crawler.HeadlessDetector,
	policy crawler.Policy,
	idGen crawler.IDGenerator,
	progressEmitter progress.Emitter,
	cfg Config,
	logger *zap.Logger,
) *Worker {
	if cfg.ContentType == "" {
		cfg.ContentType = "text/html; charset=utf-8"
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = cfg.RequestTimeout
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryBackoffBase <= 0 {
		cfg.RetryBackoffBase = 1 * time.Second
	}
	if idGen == nil {
		idGen = uuid.NewUUIDGenerator()
	}
	return &Worker{
		queue:           queue,
		jobStore:        jobStore,
		blobStore:       blobStore,
		retrievalStore:  retrievalStore,
		publisher:       publisher,
		hasher:          hasher,
		clock:           clock,
		probeFetcher:    probe,
		headlessFetcher: headless,
		detector:        detector,
		policy:          policy,
		idGen:           idGen,
		cfg:             cfg,
		logger:          logger,
		progress:        progressEmitter,
	}
}

// Run blocks, consuming queue items until the context finishes.
func (w *Worker) Run(ctx context.Context) {
	for {
		item, err := w.queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.logger.Error("queue dequeue failed", zap.Error(err))
			continue
		}
		w.logger.Debug("dequeued job", zap.String("job_id", item.JobID))
		w.processJob(ctx, item)
	}
}

func (w *Worker) processJob(ctx context.Context, item crawler.QueueItem) {
	tracer := otel.Tracer("worker")
	ctx, span := tracer.Start(ctx, "process_job", trace.WithAttributes(
		attribute.String("job_id", item.JobID),
	))
	defer span.End()

	telemetry.IncActiveWorkers()
	defer telemetry.DecActiveWorkers()

	jobCtx, cancel := w.jobContext(ctx, item.Params)
	defer cancel()

	start := w.now()
	item.JobStartedAt = start
	jobUUID, hasUUID := w.parseJobUUID(item.JobID)
	if hasUUID {
		w.emitEvent(jobUUID, progress.StageJobStart, func(evt *progress.Event) {
			evt.TS = start
		})
	}
	if w.probeFetcher == nil {
		w.logger.Error("no probe fetcher configured", zap.String("job_id", item.JobID))
		failMsg := "no probe fetcher configured"
		if err := w.updateJobStatus(ctx, item.JobID, crawler.JobStatusFailed, failMsg, crawler.JobCounters{}); err != nil {
			w.logger.Error("fail job status update", zap.String("job_id", item.JobID), zap.Error(err))
		}
		if hasUUID {
			w.emitJobCompletion(jobUUID, crawler.JobStatusFailed, start, failMsg)
		}
		return
	}
	counters := crawler.JobCounters{}
	status := crawler.JobStatusRunning
	errText := ""

	if err := w.updateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
		w.logger.Error("update job status failed", zap.String("job_id", item.JobID), zap.Error(err))
		return
	}

	for _, targetURL := range item.Params.URLs {
		if jobCtx.Err() != nil {
			errText = jobCtx.Err().Error()
			w.logger.Warn("job context canceled before URL fetch",
				zap.String("job_id", item.JobID),
				zap.String("url", targetURL),
				zap.Error(jobCtx.Err()),
			)
			break
		}
		if err := w.handleURL(jobCtx, item, jobUUID, hasUUID, targetURL, &counters); err != nil {
			errText = err.Error()
		}
	}

	status, errText = w.deriveFinalStatus(jobCtx, counters, errText)

	if err := w.updateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
		w.logger.Error("final job status update failed", zap.String("job_id", item.JobID), zap.Error(err))
	}
	telemetry.ObserveJob(string(status))
	if hasUUID {
		w.emitJobCompletion(jobUUID, status, start, errText)
	}
}

func (w *Worker) allowFetch(jobID, url string, depth int) bool {
	if w.policy == nil {
		return true
	}
	return w.policy.AllowFetch(jobID, url, depth)
}

func (w *Worker) allowHeadless(jobID, url string, depth int) bool {
	if w.policy == nil {
		return true
	}
	return w.policy.AllowHeadless(jobID, url, depth)
}

func (w *Worker) buildBlobPath(ts time.Time, host, pageID string) string {
	ts = ts.UTC()
	prefix := strings.Trim(w.cfg.BlobPrefix, "/")
	hostSegment := fmt.Sprintf("host=%s", sanitizeHost(host))
	idSegment := fmt.Sprintf("id=%s", pageID)
	datePath := ts.Format("200601") + "/" + ts.Format("02") + "/" + ts.Format("15")
	base := datePath + "/" + hostSegment + "/" + idSegment
	if prefix == "" {
		return base
	}
	return path.Join(prefix, base)
}

func (w *Worker) handleURL(
	ctx context.Context,
	item crawler.QueueItem,
	jobUUID goUUID.UUID,
	hasUUID bool,
	targetURL string,
	counters *crawler.JobCounters,
) error {
	normalizedURL, err := crawler.NormalizeURL(targetURL)
	if err != nil {
		w.logger.Warn("failed to normalize url",
			zap.String("job_id", item.JobID),
			zap.String("url", targetURL),
			zap.Error(err),
		)
	} else {
		targetURL = normalizedURL
	}

	tracer := otel.Tracer("worker")
	ctx, span := tracer.Start(ctx, "handle_url", trace.WithAttributes(
		attribute.String("job_id", item.JobID),
		attribute.String("url", targetURL),
	))
	defer span.End()

	if !w.allowFetch(item.JobID, targetURL, 0) {
		w.logger.Warn("fetch blocked by policy", zap.String("job_id", item.JobID), zap.String("url", targetURL))
		return nil
	}

	if w.policy != nil {
		if err := w.policy.Wait(ctx, targetURL); err != nil {
			w.logger.Warn("rate limiter wait failed",
				zap.String("job_id", item.JobID),
				zap.String("url", targetURL),
				zap.Error(err),
			)
			return fmt.Errorf("rate limiter wait: %w", err)
		}
	}

	var resp crawler.FetchResponse
	var fetchErr error

	resp, fetchErr = w.fetchProbeWithRetry(ctx, item, targetURL)
	if fetchErr != nil {
		counters.PagesFailed++
		w.logger.Error("probe fetch failed after retries",
			zap.String("job_id", item.JobID),
			zap.String("url", targetURL),
			zap.Error(fetchErr),
		)
		return fetchErr
	}
	w.logger.Debug("probe fetch succeeded", zap.String("job_id", item.JobID), zap.String("url", targetURL))

	finalResp := resp
	if promotedResp, promoted := w.maybePromote(ctx, item, targetURL, resp); promoted {
		finalResp = promotedResp
		w.logger.Info("headless promotion applied", zap.String("job_id", item.JobID), zap.String("url", targetURL))
	}

	if err := w.persistAndPublish(ctx, item.JobID, item.JobStartedAt, targetURL, finalResp); err != nil {
		counters.PagesFailed++
		w.logger.Error("persist page failed",
			zap.String("job_id", item.JobID),
			zap.String("url", targetURL),
			zap.Error(err),
		)
		w.recordPage(targetURL, false, len(finalResp.Body))
		return err
	}

	counters.PagesSucceeded++
	w.logger.Debug("page processed", zap.String("job_id", item.JobID), zap.String("url", targetURL))
	w.recordPage(targetURL, true, len(finalResp.Body))
	if hasUUID {
		w.emitFetchEvent(jobUUID, targetURL, finalResp)
	}
	return nil
}

func (w *Worker) fetchProbeWithRetry(
	ctx context.Context,
	item crawler.QueueItem,
	targetURL string,
) (crawler.FetchResponse, error) {
	var resp crawler.FetchResponse
	var err error

	for attempt := 0; attempt <= w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			w.logger.Info("retrying fetch",
				zap.String("job_id", item.JobID),
				zap.String("url", targetURL),
				zap.Int("attempt", attempt),
			)
			select {
			case <-ctx.Done():
				return crawler.FetchResponse{}, fmt.Errorf("context done: %w", ctx.Err())
			case <-time.After(w.cfg.RetryBackoffBase * time.Duration(attempt)):
				// Exponential backoff-ish
			}
		}

		resp, err = w.fetchProbe(ctx, item, targetURL)
		if err == nil {
			return resp, nil
		}
		// If error is context canceled, don't retry
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return crawler.FetchResponse{}, err
		}
	}
	return crawler.FetchResponse{}, err
}

func (w *Worker) fetchProbe(ctx context.Context, item crawler.QueueItem, url string) (crawler.FetchResponse, error) {
	pageCtx, cancel := w.pageContext(ctx)
	defer cancel()

	resp, err := w.probeFetcher.Fetch(pageCtx, crawler.FetchRequest{
		JobID:                 item.JobID,
		JobStartedAt:          item.JobStartedAt,
		URL:                   url,
		Depth:                 0,
		RespectRobots:         item.Params.RespectRobots,
		RespectRobotsProvided: item.Params.RespectRobotsProvided,
	})
	if err != nil {
		w.recordPage(url, false, 0)
		return crawler.FetchResponse{}, fmt.Errorf("probe fetch: %w", err)
	}
	if resp.RobotsStatus == crawler.RobotsStatusIndeterminate {
		w.logger.Warn("robots.txt probe indeterminate, defaulting to allow all",
			zap.String("job_id", item.JobID),
			zap.String("url", url),
			zap.String("phase", "probe"),
			zap.String("reason", resp.RobotsReason),
		)
	}
	return resp, nil
}

func (w *Worker) maybePromote(
	ctx context.Context,
	item crawler.QueueItem,
	url string,
	resp crawler.FetchResponse,
) (crawler.FetchResponse, bool) {
	if !item.Params.HeadlessAllowed || w.detector == nil || w.headlessFetcher == nil {
		return resp, false
	}
	if !w.allowHeadless(item.JobID, url, 0) || !w.detector.ShouldPromote(resp) {
		return resp, false
	}

	headlessCtx, cancel := w.pageContext(ctx)
	defer cancel()

	headlessResp, err := w.headlessFetcher.Fetch(headlessCtx, crawler.FetchRequest{
		JobID:                 item.JobID,
		JobStartedAt:          item.JobStartedAt,
		URL:                   url,
		Depth:                 0,
		UseHeadless:           true,
		RespectRobots:         item.Params.RespectRobots,
		RespectRobotsProvided: item.Params.RespectRobotsProvided,
	})
	if err != nil {
		w.logger.Warn("headless promotion failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
		return resp, false
	}
	headlessResp.UsedHeadless = true
	return headlessResp, true
}

func (w *Worker) persistAndPublish(
	ctx context.Context,
	jobID string,
	jobStartedAt time.Time,
	url string,
	resp crawler.FetchResponse,
) error {
	hash, err := w.hasher.Hash(resp.Body)
	if err != nil {
		return fmt.Errorf("hash body: %w", err)
	}

	pageID, err := w.newPageID()
	if err != nil {
		return fmt.Errorf("generate page id: %w", err)
	}

	storedAt := w.clock.Now().UTC()
	site := extractHost(resp.URL)
	basePath := w.buildBlobPath(storedAt, site, pageID)

	rawPath := path.Join(basePath, "raw.html")
	uri, err := w.blobStore.PutObject(ctx, rawPath, w.cfg.ContentType, bytes.NewReader(resp.Body))
	if err != nil {
		return fmt.Errorf("put raw object: %w", err)
	}

	headersPath := path.Join(basePath, "headers.json")
	if _, innerErr := w.putJSON(ctx, headersPath, normalizeHeaders(resp.Headers)); innerErr != nil {
		return fmt.Errorf("put headers: %w", innerErr)
	}

	metaPath := path.Join(basePath, "meta.json")
	contentType := contentTypeFromResponse(resp, w.cfg.ContentType)

	meta := pageMetaPayload{
		URL:         resp.URL,
		Status:      resp.StatusCode,
		Size:        len(resp.Body),
		SHA256:      hash,
		ContentType: contentType,
		ParentID:    "",
		JobUUID:     jobID,
	}
	metaURI, err := w.putJSON(ctx, metaPath, meta)
	if err != nil {
		return fmt.Errorf("put meta: %w", err)
	}

	if len(resp.Screenshot) > 0 {
		screenshotPath := path.Join(basePath, "screenshot.png")
		if _, err := w.blobStore.PutObject(ctx, screenshotPath, "image/png", bytes.NewReader(resp.Screenshot)); err != nil {
			return fmt.Errorf("put screenshot: %w", err)
		}
	}

	page := crawler.PageRecord{
		ID:           pageID,
		JobID:        jobID,
		JobStartedAt: jobStartedAt,
		URL:          resp.URL,
		StatusCode:   resp.StatusCode,
		UsedHeadless: resp.UsedHeadless,
		FetchedAt:    storedAt,
		DurationMs:   resp.Duration.Milliseconds(),
		ContentHash:  hash,
		Headers:      resp.Headers,
		BlobURI:      uri,
	}
	if err := w.jobStore.RecordPage(ctx, page); err != nil {
		return fmt.Errorf("record page: %w", err)
	}

	if err := w.storeRetrieval(ctx, page, contentType, len(resp.Body)); err != nil {
		return err
	}

	if err := w.publishResult(ctx, jobID, site, url, uri, metaURI, hash, resp, pageID); err != nil {
		return err
	}
	return nil
}

func (w *Worker) publishResult(
	ctx context.Context,
	jobID string,
	site string,
	url string,
	htmlURI string,
	metaURI string,
	hash string,
	resp crawler.FetchResponse,
	pageID string,
) error {
	if w.cfg.Topic == "" || w.publisher == nil {
		return nil
	}
	payload := map[string]any{
		"crawl_id":       pageID,
		"job_id":         jobID,
		"site":           site,
		"url":            url,
		"html_blob":      htmlURI,
		"meta_blob":      metaURI,
		"schema_version": pubSubSchemaVersion,
	}
	if _, err := w.publisher.Publish(ctx, w.cfg.Topic, payload); err != nil {
		return fmt.Errorf("publish payload: %w", err)
	}
	w.logger.Info("page published",
		zap.String("job_id", jobID),
		zap.String("page_id", pageID),
		zap.String("url", url),
		zap.String("html_blob", htmlURI),
		zap.String("meta_blob", metaURI),
		zap.String("hash", hash),
		zap.Bool("headless", resp.UsedHeadless),
	)
	return nil
}

type pageMetaPayload struct {
	URL         string `json:"url"`
	Status      int    `json:"status"`
	Size        int    `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	ParentID    string `json:"parent_id"`
	JobUUID     string `json:"job_uuid"`
}

func (w *Worker) putJSON(ctx context.Context, objectPath string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	uri, err := w.blobStore.PutObject(ctx, objectPath, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("put object: %w", err)
	}
	return uri, nil
}

func normalizeHeaders(h http.Header) map[string][]string {
	if len(h) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(h))
	for k, values := range h {
		out[k] = append([]string(nil), values...)
	}
	return out
}

func (w *Worker) newPageID() (string, error) {
	if w.idGen == nil {
		return "", fmt.Errorf("id generator is not configured")
	}
	id, err := w.idGen.NewID()
	if err != nil {
		return "", fmt.Errorf("generate page id: %w", err)
	}
	return id, nil
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}

func sanitizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-':
			b.WriteRune(r)
		case r == ':':
			b.WriteRune('_')
		default:
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-_")
	if result == "" {
		return "unknown"
	}
	return result
}

func (w *Worker) storeRetrieval(ctx context.Context, page crawler.PageRecord, contentType string, size int) error {
	if w.retrievalStore == nil {
		return nil
	}
	record := crawler.RetrievalRecord{
		ID:           page.ID,
		JobID:        page.JobID,
		JobStartedAt: page.JobStartedAt,
		URL:          page.URL,
		Hash:         page.ContentHash,
		BlobURI:      page.BlobURI,
		Headers:      page.Headers,
		StatusCode:   page.StatusCode,
		ContentType:  contentType,
		SizeBytes:    size,
		RetrievedAt:  page.FetchedAt,
		PartitionTS:  page.FetchedAt.Truncate(time.Hour),
	}
	if err := w.retrievalStore.StoreRetrieval(ctx, record); err != nil {
		return fmt.Errorf("store retrieval: %w", err)
	}
	return nil
}

func (w *Worker) parseJobUUID(jobID string) (goUUID.UUID, bool) {
	if jobID == "" {
		return goUUID.UUID{}, false
	}
	id, err := goUUID.Parse(jobID)
	if err != nil {
		w.logger.Debug("job id is not a uuid", zap.String("job_id", jobID), zap.Error(err))
		return goUUID.UUID{}, false
	}
	return id, true
}

func (w *Worker) emitJobCompletion(jobUUID goUUID.UUID, status crawler.JobStatus, started time.Time, note string) {
	stage := stageFromStatus(status)
	if stage == "" {
		return
	}
	w.emitEvent(jobUUID, stage, func(evt *progress.Event) {
		evt.Dur = w.now().Sub(started)
		if note != "" {
			evt.Note = note
		}
	})
}

func (w *Worker) emitFetchEvent(jobUUID goUUID.UUID, url string, resp crawler.FetchResponse) {
	site := extractHost(url)
	w.emitEvent(jobUUID, progress.StageFetchDone, func(evt *progress.Event) {
		evt.Site = site
		evt.URL = url
		evt.Bytes = int64(len(resp.Body))
		evt.Visits = 1
		evt.StatusClass = progress.ClassifyStatus(resp.StatusCode)
		evt.Dur = resp.Duration
	})
}

func (w *Worker) emitEvent(jobUUID goUUID.UUID, stage progress.Stage, mutate func(*progress.Event)) {
	if w.progress == nil || stage == "" || jobUUID == (goUUID.UUID{}) {
		return
	}
	evt := progress.Event{
		JobID: progress.UUIDToBytes(jobUUID),
		TS:    w.now(),
		Stage: stage,
	}
	if mutate != nil {
		mutate(&evt)
	}
	w.progress.Emit(evt)
}

func stageFromStatus(status crawler.JobStatus) progress.Stage {
	switch status {
	case crawler.JobStatusSucceeded:
		return progress.StageJobDone
	case crawler.JobStatusFailed, crawler.JobStatusCanceled:
		return progress.StageJobError
	default:
		return ""
	}
}

func (w *Worker) now() time.Time {
	if w.clock != nil {
		return w.clock.Now()
	}
	return time.Now().UTC()
}

func contentTypeFromResponse(resp crawler.FetchResponse, fallback string) string {
	if resp.Headers != nil {
		if ct := resp.Headers.Get("Content-Type"); ct != "" {
			return ct
		}
	}
	return fallback
}

func (w *Worker) deriveFinalStatus(
	ctx context.Context,
	counters crawler.JobCounters,
	errText string,
) (crawler.JobStatus, string) {
	if counters.PagesSucceeded == 0 && errText == "" {
		errText = "no pages were fetched"
	}

	switch {
	case ctx.Err() != nil:
		return crawler.JobStatusCanceled, errText
	case counters.PagesSucceeded == 0:
		return crawler.JobStatusFailed, errText
	default:
		return crawler.JobStatusSucceeded, errText
	}
}

func (w *Worker) recordPage(url string, success bool, bytes int) {
	statusStr := "success"
	if !success {
		statusStr = "error"
	}
	telemetry.ObserveCrawl(url, statusStr, bytes)
}

func (w *Worker) jobContext(
	parent context.Context,
	params crawler.JobParameters,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	budget := time.Duration(params.BudgetSeconds) * time.Second
	if budget <= 0 {
		budget = w.cfg.JobTimeout
	}
	if budget > 0 {
		return context.WithTimeout(parent, budget)
	}
	return context.WithCancel(parent)
}

func (w *Worker) pageContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if w.cfg.RequestTimeout > 0 {
		return context.WithTimeout(parent, w.cfg.RequestTimeout)
	}
	return context.WithCancel(parent)
}

func (w *Worker) updateJobStatus(
	ctx context.Context,
	jobID string,
	status crawler.JobStatus,
	errText string,
	counters crawler.JobCounters,
) error {
	if w.jobStore == nil {
		return fmt.Errorf("job store is not configured")
	}
	updateCtx, cancel := w.statusContext(ctx)
	defer cancel()
	if err := w.jobStore.UpdateJobStatus(updateCtx, jobID, status, errText, counters); err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

func (w *Worker) statusContext(_ context.Context) (context.Context, context.CancelFunc) {
	const statusUpdateTimeout = 5 * time.Second
	return context.WithTimeout(context.Background(), statusUpdateTimeout)
}
