// Package worker implements the crawl pipeline execution loop.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/id/uuid"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
)

// Config controls Worker behavior.
type Config struct {
	ContentType string
	BlobPrefix  string
	Topic       string
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
	meters          *metrics.Collectors
}

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
	cfg Config,
	logger *zap.Logger,
	meters *metrics.Collectors,
) *Worker {
	if cfg.ContentType == "" {
		cfg.ContentType = "text/html; charset=utf-8"
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
		meters:          meters,
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
	start := time.Now()
	if w.probeFetcher == nil {
		w.logger.Error("no probe fetcher configured", zap.String("job_id", item.JobID))
		if err := w.jobStore.UpdateJobStatus(
			ctx,
			item.JobID,
			crawler.JobStatusFailed,
			"no probe fetcher configured",
			crawler.JobCounters{},
		); err != nil {
			w.logger.Error("fail job status update", zap.String("job_id", item.JobID), zap.Error(err))
		}
		return
	}
	counters := crawler.JobCounters{}
	status := crawler.JobStatusRunning
	errText := ""

	if err := w.jobStore.UpdateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
		w.logger.Error("update job status failed", zap.String("job_id", item.JobID), zap.Error(err))
		return
	}

	for _, url := range item.Params.URLs {
		if err := w.handleURL(ctx, item, url, &counters); err != nil {
			errText = err.Error()
		}
	}

	status, errText = w.deriveFinalStatus(ctx, counters, errText)

	if err := w.jobStore.UpdateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
		w.logger.Error("final job status update failed", zap.String("job_id", item.JobID), zap.Error(err))
	}
	w.recordJobFinish(status, start)
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
	url string,
	counters *crawler.JobCounters,
) error {
	if !w.allowFetch(item.JobID, url, 0) {
		w.logger.Warn("fetch blocked by policy", zap.String("job_id", item.JobID), zap.String("url", url))
		return nil
	}

	resp, err := w.fetchProbe(ctx, item, url)
	if err != nil {
		counters.PagesFailed++
		w.logger.Error("probe fetch failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
		return err
	}
	w.logger.Debug("probe fetch succeeded", zap.String("job_id", item.JobID), zap.String("url", url))

	finalResp := resp
	if promotedResp, promoted := w.maybePromote(ctx, item, url, resp); promoted {
		finalResp = promotedResp
		w.logger.Info("headless promotion applied", zap.String("job_id", item.JobID), zap.String("url", url))
	}

	if err := w.persistAndPublish(ctx, item.JobID, url, finalResp); err != nil {
		counters.PagesFailed++
		w.logger.Error("persist page failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
		w.recordPage(false, finalResp.UsedHeadless, finalResp.StatusCode, finalResp.Duration)
		return err
	}

	counters.PagesSucceeded++
	w.logger.Debug("page processed", zap.String("job_id", item.JobID), zap.String("url", url))
	w.recordPage(true, finalResp.UsedHeadless, finalResp.StatusCode, finalResp.Duration)
	return nil
}

func (w *Worker) fetchProbe(ctx context.Context, item crawler.QueueItem, url string) (crawler.FetchResponse, error) {
	pageCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resp, err := w.probeFetcher.Fetch(pageCtx, crawler.FetchRequest{
		JobID:                 item.JobID,
		URL:                   url,
		Depth:                 0,
		RespectRobots:         item.Params.RespectRobots,
		RespectRobotsProvided: item.Params.RespectRobotsProvided,
	})
	if err != nil {
		w.recordPage(false, false, 0, 0)
		return crawler.FetchResponse{}, fmt.Errorf("probe fetch: %w", err)
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

	headlessCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	headlessResp, err := w.headlessFetcher.Fetch(headlessCtx, crawler.FetchRequest{
		JobID:                 item.JobID,
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

func (w *Worker) persistAndPublish(ctx context.Context, jobID, url string, resp crawler.FetchResponse) error {
	hash, err := w.hasher.Hash(resp.Body)
	if err != nil {
		return fmt.Errorf("hash body: %w", err)
	}

	pageID, err := w.newPageID()
	if err != nil {
		return fmt.Errorf("generate page id: %w", err)
	}

	storedAt := w.clock.Now().UTC()
	basePath := w.buildBlobPath(storedAt, extractHost(resp.URL), pageID)

	rawPath := path.Join(basePath, "raw.html")
	uri, err := w.blobStore.PutObject(ctx, rawPath, w.cfg.ContentType, resp.Body)
	if err != nil {
		return fmt.Errorf("put raw object: %w", err)
	}

	headersPath := path.Join(basePath, "headers.json")
	if err := w.putJSON(ctx, headersPath, normalizeHeaders(resp.Headers)); err != nil {
		return fmt.Errorf("put headers: %w", err)
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
	if err := w.putJSON(ctx, metaPath, meta); err != nil {
		return fmt.Errorf("put meta: %w", err)
	}

	if len(resp.Screenshot) > 0 {
		screenshotPath := path.Join(basePath, "screenshot.png")
		if _, err := w.blobStore.PutObject(ctx, screenshotPath, "image/png", resp.Screenshot); err != nil {
			return fmt.Errorf("put screenshot: %w", err)
		}
	}

	page := crawler.PageRecord{
		ID:           pageID,
		JobID:        jobID,
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

	if err := w.publishResult(ctx, jobID, url, uri, hash, resp, pageID); err != nil {
		return err
	}
	return nil
}

func (w *Worker) publishResult(
	ctx context.Context,
	jobID string,
	url string,
	uri string,
	hash string,
	resp crawler.FetchResponse,
	pageID string,
) error {
	if w.cfg.Topic == "" || w.publisher == nil {
		return nil
	}
	payload := map[string]any{
		"job_id":    jobID,
		"page_id":   pageID,
		"url":       url,
		"blob_uri":  uri,
		"hash":      hash,
		"timestamp": w.clock.Now().Format(time.RFC3339),
		"status":    resp.StatusCode,
		"headless":  resp.UsedHeadless,
	}
	if _, err := w.publisher.Publish(ctx, w.cfg.Topic, payload); err != nil {
		return fmt.Errorf("publish payload: %w", err)
	}
	w.logger.Info("page published",
		zap.String("job_id", jobID),
		zap.String("page_id", pageID),
		zap.String("url", url),
		zap.String("blob_uri", uri),
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

func (w *Worker) putJSON(ctx context.Context, objectPath string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := w.blobStore.PutObject(ctx, objectPath, "application/json", data); err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
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
		ID:          page.ID,
		JobID:       page.JobID,
		URL:         page.URL,
		Hash:        page.ContentHash,
		BlobURI:     page.BlobURI,
		Headers:     page.Headers,
		StatusCode:  page.StatusCode,
		ContentType: contentType,
		SizeBytes:   size,
		RetrievedAt: page.FetchedAt,
		PartitionTS: page.FetchedAt.Truncate(time.Hour),
	}
	if err := w.retrievalStore.StoreRetrieval(ctx, record); err != nil {
		return fmt.Errorf("store retrieval: %w", err)
	}
	return nil
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

func (w *Worker) recordPage(success, usedHeadless bool, status int, duration time.Duration) {
	if w.meters == nil {
		return
	}
	w.meters.PageProcessed(success, usedHeadless, status, duration)
}

func (w *Worker) recordJobFinish(status crawler.JobStatus, start time.Time) {
	if w.meters == nil {
		return
	}
	w.meters.JobFinished(status, time.Since(start))
}
