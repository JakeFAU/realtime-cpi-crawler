// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"time"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
)

// collyCrawler implements the Crawler interface using the Colly library.
type collyCrawler struct {
	config  Config
	logger  *zap.Logger
	storage storage.Provider
	db      database.Provider
	queue   queue.Provider
}

// NewCollyCrawler creates a new instance of a Colly-based crawler.
func NewCollyCrawler(
	config Config,
	logger *zap.Logger,
	storageProvider storage.Provider,
	dbProvider database.Provider,
	queueProvider queue.Provider,
) Crawler {
	return &collyCrawler{
		config:  config,
		logger:  logger,
		storage: storageProvider,
		db:      dbProvider,
		queue:   queueProvider,
	}
}

// Run starts the crawling process.
func (c *collyCrawler) Run(ctx context.Context) {
	collector := c.initCollector(ctx)

	for _, url := range c.config.InitialTargetURLs {
		if err := collector.Visit(url); err != nil {
			c.logger.Error("Failed to visit URL", zap.String("url", url), zap.Error(err))
		}
	}

	collector.Wait()
}

func (c *collyCrawler) initCollector(ctx context.Context) *colly.Collector {
	collector := colly.NewCollector(
		colly.AllowedDomains(c.config.AllowedDomains...),
		colly.MaxDepth(c.config.MaxDepth),
		colly.UserAgent(c.config.UserAgent),
		colly.Async(true),
		colly.URLFilters(c.config.URLFilters...),
	)
	collector.AllowURLRevisit = false

	if err := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: c.config.Concurrency,
		Delay:       c.config.Delay,
	}); err != nil {
		c.logger.Fatal("Failed to set collector limits", zap.Error(err))
	}

	collector.OnHTML("a[href]", c.handleHTML)
	collector.OnResponse(c.handleResponse(ctx))
	collector.OnError(c.handleError)

	return collector
}

func (c *collyCrawler) handleHTML(e *colly.HTMLElement) {
	if err := e.Request.Visit(e.Attr("href")); err != nil {
		c.logger.Error("Failed to visit link", zap.String("url", e.Attr("href")), zap.Error(err))
	}
}

func (c *collyCrawler) handleResponse(ctx context.Context) func(*colly.Response) {
	return func(r *colly.Response) {
		if r.StatusCode != 200 || len(r.Body) == 0 {
			c.logger.Warn("Skipping response",
				zap.String("url", r.Request.URL.String()),
				zap.Int("status_code", r.StatusCode),
			)
			return
		}

		blobHash := fmt.Sprintf("%x", sha256.Sum256(r.Body))
		objectName := c.generateObjectName(r.Request.URL.String(), time.Now().UTC())

		if err := c.storage.Save(ctx, objectName, r.Body); err != nil {
			c.logger.Error("Failed to save response", zap.Error(err))
			return
		}

		headers := make(map[string]any)
		for k, v := range *r.Headers {
			headers[k] = v
		}

		meta := database.CrawlMetadata{
			URL:       r.Request.URL.String(),
			FetchedAt: time.Now().UTC(),
			BlobLink:  objectName,
			BlobHash:  blobHash,
			Headers:   headers,
		}

		crawlID, err := c.db.SaveCrawl(ctx, meta)
		if err != nil {
			c.logger.Error("Failed to save crawl metadata", zap.Error(err))
			return
		}

		if err := c.queue.Publish(ctx, crawlID); err != nil {
			c.logger.Error("Failed to publish crawl notification", zap.Error(err))
		}
	}
}

func (c *collyCrawler) handleError(r *colly.Response, err error) {
	msg := "Request failed"
	switch r.StatusCode {
	case 429:
		msg = "Rate limited"
	case 403:
		msg = "Forbidden"
	}
	c.logger.Error(msg,
		zap.String("url", r.Request.URL.String()),
		zap.Int("status_code", r.StatusCode),
		zap.Error(err),
	)
}

func (c *collyCrawler) generateObjectName(url string, fetchedAt time.Time) string {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	return path.Join(
		"pages",
		fetchedAt.Format("2006-01-02"),
		fmt.Sprintf("%s.html", urlHash),
	)
}
