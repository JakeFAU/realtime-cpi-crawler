package crawler

import (
	"context"
	"net/http"
	"time"
)

// Page represents a fetched or rendered document.
type Page struct {
	URL        string
	FinalURL   string
	StatusCode int
	Headers    http.Header
	Body       []byte
	UsedJS     bool
}

// ContentLength returns the number of bytes in the page body.
func (p Page) ContentLength() int {
	return len(p.Body)
}

// IsHTML returns true when the Content-Type header indicates HTML.
func (p Page) IsHTML() bool {
	ct := p.Headers.Get("Content-Type")
	return ct == "" || containsLower(ct, "text/html")
}

// Fetcher retrieves pages without executing JavaScript.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (Page, error)
}

// Renderer produces fully rendered DOM snapshots with JavaScript execution.
type Renderer interface {
	Render(ctx context.Context, rawURL string) (Page, error)
	Close(ctx context.Context) error
}

// Detector decides whether a fast-path HTML body needs JS rendering.
type Detector interface {
	NeedsJS(ctx context.Context, page Page) bool
}

// RobotsPolicy determines whether a URL may be crawled.
type RobotsPolicy interface {
	Allowed(ctx context.Context, rawURL string) bool
}

// RetryPolicy encapsulates retry behavior for transient errors.
type RetryPolicy interface {
	ShouldRetry(err error, attempt int) bool
	Backoff(attempt int) time.Duration
}

// StorageSink persists page bodies and crawl metadata.
type StorageSink interface {
	SaveHTML(ctx context.Context, page Page) (string, error)
	SaveMeta(ctx context.Context, meta CrawlMetadata) error
}

// CrawlMetadata captures information stored alongside each page snapshot.
type CrawlMetadata struct {
	URL       string    `json:"url"`
	FinalURL  string    `json:"final_url"`
	Status    int       `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	UsedJS    bool      `json:"used_js"`
	ByteSize  int       `json:"byte_size"`
	Path      string    `json:"path"`
}
