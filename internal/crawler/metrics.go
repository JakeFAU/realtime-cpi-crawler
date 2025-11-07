// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TotalScrapes tracks the number of pages successfully scraped and persisted.
	TotalScrapes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crawler_scrapes_total",
		Help: "The total number of pages successfully scraped and saved.",
	})
	// TotalRequests tracks the number of HTTP requests dispatched by the crawler.
	TotalRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crawler_requests_total",
		Help: "The total number of HTTP requests sent.",
	})
	// TotalRequestErrors tracks the number of requests that resulted in an error.
	TotalRequestErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crawler_request_errors_total",
		Help: "The total number of failed HTTP requests.",
	})
	// TotalRateLimitHits tracks the number of times the crawler was rate-limited (HTTP 429).
	TotalRateLimitHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crawler_rate_limit_hits_total",
		Help: "The total number of times the crawler was rate limited.",
	})
	// TotalForbiddenHits tracks the number of times the crawler received a forbidden response (HTTP 403).
	TotalForbiddenHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crawler_forbidden_hits_total",
		Help: "The total number of times the crawler received a forbidden response.",
	})
)
