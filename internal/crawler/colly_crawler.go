// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
)

const (
	defaultRateLimitBackoff  = 5 * time.Second
	defaultForbiddenAttempts = 3
)

// collyCrawler implements the Crawler interface using the Colly library.
// It orchestrates the crawling process, including handling retries, backoff, and data persistence.
type collyCrawler struct {
	config  Config
	logger  *zap.Logger
	storage storage.Provider
	db      database.Provider
	queue   queue.Provider

	visits    visitTracker
	domains   domainBlocker
	blocklist *domainPatternBlocklist
	pauser    pauseController
}

// NewCollyCrawler creates a new instance of a Colly-based crawler.
// It takes the crawler configuration and the application's service providers as input.
// It returns a configured Crawler interface, ready to be started.
func NewCollyCrawler(
	config Config,
	logger *zap.Logger,
	storageProvider storage.Provider,
	dbProvider database.Provider,
	queueProvider queue.Provider,
) Crawler {
	cfg := applyCrawlerDefaults(config)
	return &collyCrawler{
		config:  cfg,
		logger:  logger,
		storage: storageProvider,
		db:      dbProvider,
		queue:   queueProvider,
		visits:  newConcurrentVisitTracker(),
		domains: newThresholdDomainBlocker(cfg.MaxForbiddenResponses),
		blocklist: newDomainPatternBlocklist(
			cfg.BlockedDomains,
		),
		pauser: &timerPauseController{},
	}
}

// Run starts the crawling process.
// It initializes the Colly collector and starts the crawl from the seed URLs.
// This is a blocking call that will continue until the crawl is complete or the context is cancelled.
func (c *collyCrawler) Run(ctx context.Context) {
	collector := c.initCollector(ctx)

	for _, seed := range c.config.InitialTargetURLs {
		c.scheduleVisit(seed, func(u string) error { return collector.Visit(u) })
	}

	collector.Wait()
}

func (c *collyCrawler) initCollector(ctx context.Context) *colly.Collector {
	opts := []colly.CollectorOption{
		colly.MaxDepth(c.config.MaxDepth),
		colly.UserAgent(c.config.UserAgent),
		colly.Async(true),
	}
	if len(c.config.URLFilters) > 0 {
		opts = append(opts, colly.URLFilters(c.config.URLFilters...))
	}

	collector := colly.NewCollector(opts...)
	if c.config.IgnoreRobots {
		c.logger.Warn("Ignoring robots.txt rules as per configuration")
		c.logger.Info("Be respectful of website policies and consider the ethical implications of web crawling.")
		collector.IgnoreRobotsTxt = true
	} else {
		c.logger.Info("Robots.txt enforcement enabled; crawler will respect site directives.")
	}
	collector.AllowURLRevisit = false

	if c.config.HTTPTransport != nil {
		collector.WithTransport(c.config.HTTPTransport)
	}

	if c.config.HTTPTimeout > 0 {
		collector.SetRequestTimeout(c.config.HTTPTimeout)
	}

	if err := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: c.config.Concurrency,
		Delay:       c.config.Delay,
	}); err != nil {
		c.logger.Fatal("Failed to set collector limits", zap.Error(err))
	}

	collector.OnRequest(func(r *colly.Request) {
		TotalRequests.Inc()
		c.logger.Debug("Dispatching request", zap.String("url", r.URL.String()))
	})
	collector.OnHTML("a[href]", c.handleHTML)
	collector.OnResponse(c.handleResponse(ctx))
	collector.OnError(c.handleError(ctx))

	return collector
}

func (c *collyCrawler) handleHTML(e *colly.HTMLElement) {
	href := e.Attr("href")
	absoluteURL := e.Request.AbsoluteURL(href)
	if absoluteURL == "" {
		c.logger.Debug("Skipping empty link attribute", zap.String("href", href))
		return
	}
	c.scheduleVisit(absoluteURL, e.Request.Visit)
}

func (c *collyCrawler) handleResponse(ctx context.Context) func(*colly.Response) {
	return func(r *colly.Response) {
		urlStr, ok := c.validateResponse(r)
		if !ok {
			return
		}

		if err := c.persistResponse(ctx, r, urlStr); err != nil {
			c.logger.Error("Failed to persist crawl result",
				zap.String("url", urlStr),
				zap.Error(err),
			)
		}
	}
}

func (c *collyCrawler) handleError(ctx context.Context) func(*colly.Response, error) {
	return func(r *colly.Response, err error) {
		TotalRequestErrors.Inc()
		if r == nil || r.Request == nil || r.Request.URL == nil {
			c.logger.Error("Request failed before response was received", zap.Error(err))
			return
		}

		urlStr := r.Request.URL.String()
		fields := []zap.Field{
			zap.String("url", urlStr),
			zap.Int("status_code", r.StatusCode),
		}
		if err != nil {
			fields = append(fields, zap.Error(err))
		}

		switch r.StatusCode {
		case http.StatusTooManyRequests:
			TotalRateLimitHits.Inc()
			c.logger.Warn("Rate limited; backing off before retrying", fields...)
			c.pauser.Pause(ctx, c.config.RateLimitBackoff)
		case http.StatusForbidden:
			TotalForbiddenHits.Inc()
			host := r.Request.URL.Host
			blocked := c.domains.MarkForbidden(host)
			fields = append(fields, zap.String("domain", host), zap.Bool("domain_blocked", blocked))
			c.logger.Warn("Forbidden response received", fields...)
		default:
			c.logger.Error("Request failed", fields...)
		}
	}
}

func (c *collyCrawler) generateObjectName(url string, fetchedAt time.Time) string {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	return path.Join(
		"pages",
		fetchedAt.Format("2006-01-02"),
		fmt.Sprintf("%s.html", urlHash),
	)
}

func (c *collyCrawler) scheduleVisit(rawURL string, visitFn func(string) error) {
	if !c.shouldVisit(rawURL) {
		return
	}
	if err := visitFn(rawURL); err != nil {
		c.logger.Error("Failed to schedule visit", zap.String("url", rawURL), zap.Error(err))
	}
}

func (c *collyCrawler) shouldVisit(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		c.logger.Warn("Skipping malformed URL", zap.String("url", rawURL), zap.Error(err))
		return false
	}
	if host := parsed.Hostname(); host != "" && c.blocklist != nil && c.blocklist.IsBlocked(host) {
		c.logger.Info("Skipping domain blocked by configuration", zap.String("url", rawURL), zap.String("host", host))
		return false
	}
	if c.domains.IsBlocked(parsed.Host) {
		c.logger.Info("Skipping blocked domain", zap.String("url", rawURL))
		return false
	}
	if !c.visits.MarkIfNew(rawURL) {
		c.logger.Debug("URL already scheduled", zap.String("url", rawURL))
		return false
	}
	return true
}

func applyCrawlerDefaults(cfg Config) Config {
	if cfg.RateLimitBackoff <= 0 {
		cfg.RateLimitBackoff = defaultRateLimitBackoff
	}
	if cfg.MaxForbiddenResponses <= 0 {
		cfg.MaxForbiddenResponses = defaultForbiddenAttempts
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	return cfg
}

func (c *collyCrawler) validateResponse(r *colly.Response) (string, bool) {
	if r == nil || r.Request == nil || r.Request.URL == nil {
		c.logger.Warn("Received nil response, skipping persistence step.")
		return "", false
	}

	urlStr := r.Request.URL.String()
	if r.StatusCode != http.StatusOK {
		c.logger.Warn("Skipping non-OK response",
			zap.String("url", urlStr),
			zap.Int("status_code", r.StatusCode),
		)
		return "", false
	}

	if len(r.Body) == 0 {
		c.logger.Warn("Skipping response with empty body", zap.String("url", urlStr))
		return "", false
	}
	return urlStr, true
}

func (c *collyCrawler) persistResponse(ctx context.Context, r *colly.Response, urlStr string) error {
	fetchedAt := time.Now().UTC()
	blobHash := fmt.Sprintf("%x", sha256.Sum256(r.Body))
	objectName := c.generateObjectName(urlStr, fetchedAt)

	if err := c.storage.Save(ctx, objectName, r.Body); err != nil {
		return fmt.Errorf("save blob to storage: %w", err)
	}

	meta := database.CrawlMetadata{
		URL:       urlStr,
		FetchedAt: fetchedAt,
		BlobLink:  objectName,
		BlobHash:  blobHash,
		Headers:   cloneHeaders(r.Headers),
	}

	crawlID, err := c.db.SaveCrawl(ctx, meta)
	if err != nil {
		return fmt.Errorf("save crawl metadata: %w", err)
	}

	if err := c.queue.Publish(ctx, crawlID); err != nil {
		return fmt.Errorf("publish crawl notification: %w", err)
	}

	c.logger.Info("Crawl successfully recorded",
		zap.String("url", urlStr),
		zap.String("blob_link", objectName),
		zap.String("blob_hash", blobHash),
	)
	TotalScrapes.Inc()
	return nil
}

func cloneHeaders(headers *http.Header) map[string]any {
	result := make(map[string]any)
	if headers == nil {
		return result
	}
	for k, v := range *headers {
		result[k] = v
	}
	return result
}
