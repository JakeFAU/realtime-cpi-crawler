// Package worker implements the crawl pipeline execution loop.
package worker

import (
	"context"
	"fmt"
<<<<<<< HEAD
	"strings"
	"time"

	"go.uber.org/zap"

=======
	"log/slog"
	"strings"
	"time"

>>>>>>> b22344a4 (refactor to server)
	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
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
	publisher       crawler.Publisher
	hasher          crawler.Hasher
	clock           crawler.Clock
	probeFetcher    crawler.Fetcher
	headlessFetcher crawler.Fetcher
	detector        crawler.HeadlessDetector
	policy          crawler.Policy
	cfg             Config
<<<<<<< HEAD
	logger          *zap.Logger
=======
	logger          *slog.Logger
>>>>>>> b22344a4 (refactor to server)
}

// New constructs a Worker.
func New(
	queue crawler.Queue,
	jobStore crawler.JobStore,
	blobStore crawler.BlobStore,
	publisher crawler.Publisher,
	hasher crawler.Hasher,
	clock crawler.Clock,
	probe crawler.Fetcher,
	headless crawler.Fetcher,
	detector crawler.HeadlessDetector,
	policy crawler.Policy,
	cfg Config,
<<<<<<< HEAD
	logger *zap.Logger,
) *Worker {
=======
	logger *slog.Logger,
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
>>>>>>> b22344a4 (refactor to server)
	if cfg.ContentType == "" {
		cfg.ContentType = "text/html; charset=utf-8"
	}
	return &Worker{
		queue:           queue,
		jobStore:        jobStore,
		blobStore:       blobStore,
		publisher:       publisher,
		hasher:          hasher,
		clock:           clock,
		probeFetcher:    probe,
		headlessFetcher: headless,
		detector:        detector,
		policy:          policy,
		cfg:             cfg,
		logger:          logger,
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
<<<<<<< HEAD
			w.logger.Error("queue dequeue failed", zap.Error(err))
			continue
		}
		w.logger.Debug("dequeued job", zap.String("job_id", item.JobID))
=======
			w.logger.Error("dequeue failed", "error", err)
			continue
		}
>>>>>>> b22344a4 (refactor to server)
		w.processJob(ctx, item)
	}
}

func (w *Worker) processJob(ctx context.Context, item crawler.QueueItem) {
	if w.probeFetcher == nil {
<<<<<<< HEAD
		w.logger.Error("no probe fetcher configured", zap.String("job_id", item.JobID))
=======
		w.logger.Error("no probe fetcher configured", "job_id", item.JobID)
>>>>>>> b22344a4 (refactor to server)
		if err := w.jobStore.UpdateJobStatus(
			ctx,
			item.JobID,
			crawler.JobStatusFailed,
			"no probe fetcher configured",
			crawler.JobCounters{},
		); err != nil {
<<<<<<< HEAD
			w.logger.Error("fail job status update", zap.String("job_id", item.JobID), zap.Error(err))
=======
			w.logger.Error("fail job status update", "job_id", item.JobID, "error", err)
>>>>>>> b22344a4 (refactor to server)
		}
		return
	}
	counters := crawler.JobCounters{}
	status := crawler.JobStatusRunning
	errText := ""

	if err := w.jobStore.UpdateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
<<<<<<< HEAD
		w.logger.Error("update job status failed", zap.String("job_id", item.JobID), zap.Error(err))
=======
		w.logger.Error("update job status", "job_id", item.JobID, "error", err)
>>>>>>> b22344a4 (refactor to server)
		return
	}

	for _, url := range item.Params.URLs {
		if err := w.handleURL(ctx, item, url, &counters); err != nil {
			errText = err.Error()
		}
	}

	status, errText = w.deriveFinalStatus(ctx, counters, errText)

	if err := w.jobStore.UpdateJobStatus(ctx, item.JobID, status, errText, counters); err != nil {
<<<<<<< HEAD
		w.logger.Error("final job status update failed", zap.String("job_id", item.JobID), zap.Error(err))
=======
		w.logger.Error("final job status update failed", "job_id", item.JobID, "error", err)
>>>>>>> b22344a4 (refactor to server)
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

func (w *Worker) buildBlobPath(jobID, hash string) string {
	prefix := strings.Trim(w.cfg.BlobPrefix, "/")
	if prefix == "" {
		return fmt.Sprintf("%s/%s.html", jobID, hash)
	}
	return fmt.Sprintf("%s/%s/%s.html", prefix, jobID, hash)
}

func (w *Worker) handleURL(
	ctx context.Context,
	item crawler.QueueItem,
	url string,
	counters *crawler.JobCounters,
) error {
	if !w.allowFetch(item.JobID, url, 0) {
<<<<<<< HEAD
		w.logger.Warn("fetch blocked by policy", zap.String("job_id", item.JobID), zap.String("url", url))
=======
		w.logger.Warn("fetch blocked by policy", "job_id", item.JobID, "url", url)
>>>>>>> b22344a4 (refactor to server)
		return nil
	}

	resp, err := w.fetchProbe(ctx, item, url)
	if err != nil {
		counters.PagesFailed++
<<<<<<< HEAD
		w.logger.Error("probe fetch failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
		return err
	}
	w.logger.Debug("probe fetch succeeded", zap.String("job_id", item.JobID), zap.String("url", url))
=======
		w.logger.Error("probe fetch failed", "job_id", item.JobID, "url", url, "error", err)
		return err
	}
>>>>>>> b22344a4 (refactor to server)

	finalResp := resp
	if promotedResp, promoted := w.maybePromote(ctx, item, url, resp); promoted {
		finalResp = promotedResp
<<<<<<< HEAD
		w.logger.Info("headless promotion applied", zap.String("job_id", item.JobID), zap.String("url", url))
=======
>>>>>>> b22344a4 (refactor to server)
	}

	if err := w.persistAndPublish(ctx, item.JobID, url, finalResp); err != nil {
		counters.PagesFailed++
<<<<<<< HEAD
		w.logger.Error("persist page failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
=======
		w.logger.Error("persist page failed", "job_id", item.JobID, "url", url, "error", err)
>>>>>>> b22344a4 (refactor to server)
		return err
	}

	counters.PagesSucceeded++
<<<<<<< HEAD
	w.logger.Debug("page processed", zap.String("job_id", item.JobID), zap.String("url", url))
=======
>>>>>>> b22344a4 (refactor to server)
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
<<<<<<< HEAD
		w.logger.Warn("headless promotion failed", zap.String("job_id", item.JobID), zap.String("url", url), zap.Error(err))
=======
		w.logger.Warn("headless promotion failed", "job_id", item.JobID, "url", url, "error", err)
>>>>>>> b22344a4 (refactor to server)
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

	blobPath := w.buildBlobPath(jobID, hash)
	uri, err := w.blobStore.PutObject(ctx, blobPath, w.cfg.ContentType, resp.Body)
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	page := crawler.PageRecord{
		JobID:        jobID,
		URL:          resp.URL,
		StatusCode:   resp.StatusCode,
		UsedHeadless: resp.UsedHeadless,
		FetchedAt:    w.clock.Now(),
		DurationMs:   resp.Duration.Milliseconds(),
		ContentHash:  hash,
		Headers:      resp.Headers,
		BlobURI:      uri,
	}
	if err := w.jobStore.RecordPage(ctx, page); err != nil {
		return fmt.Errorf("record page: %w", err)
	}

	if err := w.publishResult(ctx, jobID, url, uri, hash, resp); err != nil {
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
) error {
	if w.cfg.Topic == "" || w.publisher == nil {
		return nil
	}
	payload := map[string]any{
		"job_id":    jobID,
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
<<<<<<< HEAD
	w.logger.Info("page published",
		zap.String("job_id", jobID),
		zap.String("url", url),
		zap.String("blob_uri", uri),
		zap.String("hash", hash),
		zap.Bool("headless", resp.UsedHeadless),
	)
=======
>>>>>>> b22344a4 (refactor to server)
	return nil
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
