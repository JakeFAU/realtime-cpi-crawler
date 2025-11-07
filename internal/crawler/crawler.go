// Package crawler defines the core types and interfaces for the web crawling engine.
// It includes the definitions for crawl jobs, results, and the main Crawler orchestrator.
package crawler

import (
	"context"
	"net/http"
	"regexp"
	"time"
)

// Config holds the settings for a crawl session.
// This struct is decoupled from Viper, making the crawler and its configuration
// more modular and easier to test independently.
type Config struct {
	AllowedDomains        []string
	UserAgent             string
	HTTPTimeout           time.Duration
	MaxDepth              int
	InitialTargetURLs     []string
	Concurrency           int
	URLFilters            []*regexp.Regexp
	Delay                 time.Duration
	IgnoreRobots          bool
	HTTPTransport         http.RoundTripper
	RateLimitBackoff      time.Duration
	MaxForbiddenResponses int
}

// Crawler defines the interface for a web crawler.
type Crawler interface {
	Run(ctx context.Context)
}
