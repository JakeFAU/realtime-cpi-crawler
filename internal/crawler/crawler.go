// Package crawler defines the core types and interfaces for the web crawling engine.
// It includes the definitions for crawl jobs, results, and the main Crawler orchestrator.
package crawler

import (
	"context"
	"net/http"
	"regexp"
	"time"
)

// Config holds all the settings for a web crawl session.
// This struct is populated from the application's configuration (e.g., Viper)
// and passed to the crawler instance. It defines the behavior of the crawl,
// including target domains, concurrency limits, and politeness settings.
type Config struct {
	AllowedDomains        []string          // List of domains the crawler is allowed to visit.
	UserAgent             string            // The User-Agent string to use for HTTP requests.
	HTTPTimeout           time.Duration     // Timeout for HTTP requests.
	MaxDepth              int               // Maximum depth of links to follow.
	InitialTargetURLs     []string          // The starting URLs for the crawl.
	Concurrency           int               // The number of concurrent requests to make.
	URLFilters            []*regexp.Regexp  // A list of regex patterns to filter URLs.
	Delay                 time.Duration     // The delay between requests to the same domain.
	IgnoreRobots          bool              // Whether to ignore the robots.txt file.
	HTTPTransport         http.RoundTripper // Custom HTTP transport for requests.
	RateLimitBackoff      time.Duration     // The time to backoff when a rate limit is detected.
	MaxForbiddenResponses int               // The maximum number of consecutive forbidden responses before stopping.
}

// Crawler defines the interface for a web crawling engine.
// It provides a single method, Run, which starts the crawling process.
// The context passed to Run can be used to cancel the crawl.
type Crawler interface {
	// Run starts the crawler. It is a blocking call that will continue until the
	// crawl is complete or the context is cancelled.
	Run(ctx context.Context)
}
