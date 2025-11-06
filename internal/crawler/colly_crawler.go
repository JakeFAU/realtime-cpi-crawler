// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"regexp"
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
	collector := colly.NewCollector(
		colly.AllowedDomains(c.config.AllowedDomains...),
		colly.MaxDepth(c.config.MaxDepth),
		colly.UserAgent(c.config.UserAgent),
		colly.Async(true),
	)

	err := collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: c.config.Concurrency,
		Delay:       1 * time.Second, // Add a delay to be polite
	})
	if err != nil {
		c.logger.Fatal("Failed to set collector limits", zap.Error(err))
	}

	// Find and visit all links
	collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		err := e.Request.Visit(e.Attr("href"))
		if err != nil {
			c.logger.Error("Failed to visit link", zap.String("url", e.Attr("href")), zap.Error(err))
		}
	})

	// Regex to find prices
	priceRegex := regexp.MustCompile(`\$?\d{1,3}(?:,?\d{3})*(?:\.\d{2})?`)

	// Find prices in the HTML
	collector.OnHTML("html", func(e *colly.HTMLElement) {
		prices := priceRegex.FindAllString(e.Text, -1)
		if len(prices) > 0 {
			c.logger.Info("Found prices", zap.String("url", e.Request.URL.String()), zap.Strings("prices", prices))
		}
	})

	collector.OnResponse(func(r *colly.Response) {
		objectName := c.generateObjectName(r.Request.URL.String(), time.Now().UTC())
		if err := c.storage.Save(ctx, objectName, r.Body); err != nil {
			c.logger.Error("Failed to save response", zap.Error(err))
			return
		}

		meta := database.CrawlMetadata{
			URL:        r.Request.URL.String(),
			StorageKey: objectName,
			FetchedAt:  time.Now().UTC(),
			Metadata:   make(map[string]any),
		}

		crawlID, err := c.db.SaveCrawl(ctx, meta)
		if err != nil {
			c.logger.Error("Failed to save crawl metadata", zap.Error(err))
			return
		}

		if err := c.queue.Publish(ctx, crawlID); err != nil {
			c.logger.Error("Failed to publish crawl notification", zap.Error(err))
		}
	})

	collector.OnError(func(r *colly.Response, err error) {
		c.logger.Error("Request failed",
			zap.String("url", r.Request.URL.String()),
			zap.Int("status_code", r.StatusCode),
			zap.Error(err),
		)
	})

	for _, url := range c.config.InitialTargetURLs {
		if err := collector.Visit(url); err != nil {
			c.logger.Error("Failed to visit URL", zap.String("url", url), zap.Error(err))
		}
	}

	collector.Wait()
}

func (c *collyCrawler) generateObjectName(url string, fetchedAt time.Time) string {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	return path.Join(
		"pages",
		fetchedAt.Format("2006-01-02"),
		fmt.Sprintf("%s.html", urlHash),
	)
}
