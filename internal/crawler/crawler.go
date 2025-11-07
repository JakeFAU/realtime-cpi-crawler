package crawler

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"
)

// Engine orchestrates the crawling flow.
type Engine struct {
	cfg      CrawlerConfig
	fetcher  Fetcher
	renderer Renderer
	detector Detector
	sink     StorageSink
	robots   RobotsPolicy
	retry    RetryPolicy
	logger   *zap.Logger
	metrics  *crawlMetrics
}

// NewEngine wires all dependencies together.
func NewEngine(cfg CrawlerConfig, fetcher Fetcher, renderer Renderer, detector Detector, sink StorageSink, robots RobotsPolicy, retry RetryPolicy, logger *zap.Logger) *Engine {
	if retry == nil {
		retry = NewExponentialRetryPolicy()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Engine{
		cfg:      cfg,
		fetcher:  fetcher,
		renderer: renderer,
		detector: detector,
		sink:     sink,
		robots:   robots,
		retry:    retry,
		logger:   logger,
		metrics:  &crawlMetrics{},
	}
}

// Run executes the crawl with the configured dependencies.
func (e *Engine) Run(ctx context.Context) error {
	if e == nil {
		return errors.New("engine is nil")
	}
	jobCh := make(chan crawlJob, e.cfg.Concurrency*4)
	var jobWG sync.WaitGroup
	var workerWG sync.WaitGroup
	seen := newSeenSet()

	enqueue := func(job crawlJob) {
		if !seen.Mark(job.url) {
			return
		}
		jobWG.Add(1)
		select {
		case jobCh <- job:
		case <-ctx.Done():
			jobWG.Done()
		}
	}

	for _, seed := range e.cfg.Seeds {
		clean, parsed, err := canonicalizeURL(seed)
		if err != nil {
			e.logger.Warn("Skipping malformed seed", zap.String("seed", seed), zap.Error(err))
			continue
		}
		enqueue(crawlJob{url: clean, depth: 0, seedHost: parsed.Hostname()})
	}

	for i := 0; i < e.cfg.Concurrency; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for job := range jobCh {
				e.processJob(ctx, job, enqueue)
				jobWG.Done()
			}
		}()
	}

	go func() {
		jobWG.Wait()
		close(jobCh)
	}()

	workerWG.Wait()
	e.metrics.log(e.logger)
	return ctx.Err()
}

// Close shuts down long-lived dependencies (renderer).
func (e *Engine) Close(ctx context.Context) error {
	if e == nil || e.renderer == nil {
		return nil
	}
	return e.renderer.Close(ctx)
}

func (e *Engine) processJob(ctx context.Context, job crawlJob, enqueue func(crawlJob)) {
	if err := ctx.Err(); err != nil {
		return
	}
	if e.robots != nil && !e.robots.Allowed(ctx, job.url) {
		e.metrics.renderSkips.Add(1)
		e.logger.Info("Robots disallow URL", zap.String("url", job.url))
		return
	}

	page, err := e.fetchWithRetry(ctx, job.url)
	if err != nil {
		e.logger.Error("Fetch failed", zap.String("url", job.url), zap.Error(err))
		e.metrics.errors.Add(1)
		return
	}

	finalPage := page
	usedJS := false
	if e.shouldRender(ctx, page, job) {
		e.metrics.renderAttempts.Add(1)
		rendered, err := e.render(ctx, job.url)
		if err != nil {
			e.logger.Warn("Render failed", zap.String("url", job.url), zap.Error(err))
			e.metrics.renderErrors.Add(1)
		} else if len(rendered.Body) > len(page.Body) {
			finalPage = rendered
			usedJS = true
			e.metrics.renderSuccess.Add(1)
		} else {
			e.metrics.renderSkips.Add(1)
		}
	}
	finalPage.UsedJS = usedJS

	path, err := e.sink.SaveHTML(ctx, finalPage)
	if err != nil {
		e.logger.Error("Save HTML failed", zap.String("url", finalPage.FinalURL), zap.Error(err))
		e.metrics.errors.Add(1)
		return
	}
	meta := CrawlMetadata{
		URL:       finalPage.URL,
		FinalURL:  finalPage.FinalURL,
		Status:    finalPage.StatusCode,
		Timestamp: nowUTC(),
		UsedJS:    finalPage.UsedJS,
		ByteSize:  len(finalPage.Body),
		Path:      path,
	}
	if err := e.sink.SaveMeta(ctx, meta); err != nil {
		e.logger.Error("Save metadata failed", zap.String("url", finalPage.FinalURL), zap.Error(err))
		e.metrics.errors.Add(1)
	}
	e.metrics.stored.Add(1)

	if job.depth >= e.cfg.MaxDepth {
		return
	}
	if !finalPage.IsHTML() {
		return
	}
	for _, child := range extractLinks(finalPage) {
		clean, parsed, err := canonicalizeURL(child)
		if err != nil {
			continue
		}
		if e.cfg.EscalateOnlySameHost && parsed.Hostname() != job.seedHost {
			// Still crawl new host; the same-host rule only affects rendering.
		}
		enqueue(crawlJob{
			url:      clean,
			depth:    job.depth + 1,
			seedHost: job.seedHost,
		})
	}
}

func (e *Engine) fetchWithRetry(ctx context.Context, rawURL string) (Page, error) {
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, e.cfg.RequestTimeout)
		page, err := e.fetcher.Fetch(attemptCtx, rawURL)
		cancel()
		if err == nil {
			return page, nil
		}
		if e.retry == nil || !e.retry.ShouldRetry(err, attempt) {
			return Page{}, err
		}
		backoff := e.retry.Backoff(attempt)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return Page{}, ctx.Err()
		}
	}
}

func (e *Engine) render(ctx context.Context, rawURL string) (Page, error) {
	if e.renderer == nil {
		return Page{}, ErrRendererDisabled
	}
	renderCtx, cancel := context.WithTimeout(ctx, e.cfg.JSRenderTimeout)
	defer cancel()
	return e.renderer.Render(renderCtx, rawURL)
}

func (e *Engine) shouldRender(ctx context.Context, page Page, job crawlJob) bool {
	if !e.cfg.FeatureRenderEnabled || e.renderer == nil {
		return false
	}
	if !page.IsHTML() {
		return false
	}
	if e.cfg.EscalateOnlySameHost {
		_, parsed, err := canonicalizeURL(page.FinalURL)
		if err == nil && !strings.EqualFold(parsed.Hostname(), job.seedHost) {
			return false
		}
	}
	return e.detector != nil && e.detector.NeedsJS(ctx, page)
}

type crawlJob struct {
	url      string
	depth    int
	seedHost string
}

type seenSet struct {
	mu  sync.Mutex
	set map[string]struct{}
}

func newSeenSet() *seenSet {
	return &seenSet{set: make(map[string]struct{})}
}

func (s *seenSet) Mark(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.set[url]; ok {
		return false
	}
	s.set[url] = struct{}{}
	return true
}

type crawlMetrics struct {
	renderAttempts atomic.Int64
	renderSuccess  atomic.Int64
	renderSkips    atomic.Int64
	renderErrors   atomic.Int64
	stored         atomic.Int64
	errors         atomic.Int64
}

func (m *crawlMetrics) log(logger *zap.Logger) {
	logger.Info("Crawl complete",
		zap.Int64("render_attempts", m.renderAttempts.Load()),
		zap.Int64("render_successes", m.renderSuccess.Load()),
		zap.Int64("render_skips", m.renderSkips.Load()),
		zap.Int64("render_errors", m.renderErrors.Load()),
		zap.Int64("pages_stored", m.stored.Load()),
		zap.Int64("errors", m.errors.Load()),
	)
}

func extractLinks(page Page) []string {
	if len(page.Body) == 0 {
		return nil
	}
	baseURL := page.FinalURL
	if baseURL == "" {
		baseURL = page.URL
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(page.Body))
	if err != nil {
		return nil
	}
	results := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok || href == "" {
			return
		}
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		abs := base.ResolveReference(ref)
		if abs == nil {
			return
		}
		if !strings.EqualFold(abs.Scheme, "http") && !strings.EqualFold(abs.Scheme, "https") {
			return
		}
		abs.Fragment = ""
		results = append(results, abs.String())
	})
	return results
}
