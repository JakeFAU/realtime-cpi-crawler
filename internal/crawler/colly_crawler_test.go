// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/gocolly/colly/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func htmlResponse(req *http.Request, status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	return resp
}

func newTestConfig(t *testing.T, targetURL string, transport http.RoundTripper) Config {
	t.Helper()
	parsed, err := url.Parse(targetURL)
	require.NoError(t, err)
	prefix := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	return Config{
		URLFilters:            []*regexp.Regexp{regexp.MustCompile(regexp.QuoteMeta(prefix) + ".*")},
		UserAgent:             "test-agent",
		HTTPTimeout:           10 * time.Second,
		MaxDepth:              1,
		InitialTargetURLs:     []string{targetURL},
		Concurrency:           1,
		Delay:                 0,
		HTTPTransport:         transport,
		MaxForbiddenResponses: 2,
		RateLimitBackoff:      time.Nanosecond,
	}
}

func newInstrumentedCrawler(t *testing.T, cfg Config) *collyCrawler {
	t.Helper()
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(
		cfg,
		logger,
		&storage.NoOpProvider{},
		&database.NoOpProvider{},
		&queue.NoOpProvider{},
	)
	collyInstance, ok := crawler.(*collyCrawler)
	require.True(t, ok, "crawler should be a *collyCrawler")
	return collyInstance
}

func TestCollyCrawler_Run(t *testing.T) {
	targetURL := "https://example.org/page"
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, targetURL, req.URL.String())
			return htmlResponse(req, http.StatusOK, `<html><body>Hello</body></html>`), nil
		},
	}

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(nil)

	// 4. Create crawler config
	cfg := newTestConfig(t, targetURL, transport)

	// 5. Create and run the crawler
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

func TestCollyCrawler_Run_OnError(t *testing.T) {
	targetURL := "https://example.org/error"
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			require.Equal(t, targetURL, req.URL.String())
			return htmlResponse(req, http.StatusInternalServerError, ``), nil
		},
	}

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Create crawler config
	cfg := newTestConfig(t, targetURL, transport)

	// 4. Create and run the crawler
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 5. Assert that the mocks were not called because of the HTTP error
	mockStorage.AssertNotCalled(t, "Save", mock.Anything, mock.Anything, mock.Anything)
	mockDB.AssertNotCalled(t, "SaveCrawl", mock.Anything, mock.Anything)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_StorageError(t *testing.T) {
	targetURL := "https://example.org/storage"
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			return htmlResponse(req, http.StatusOK, `<html><body>Data</body></html>`), nil
		},
	}

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On(
		"Save",
		mock.Anything, mock.AnythingOfType("string"),
		mock.AnythingOfType("[]uint8")).Return(fmt.Errorf("storage error"))

	// 4. Create crawler config
	cfg := newTestConfig(t, targetURL, transport)

	// 5. Create and run the crawler
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertNotCalled(t, "SaveCrawl", mock.Anything, mock.Anything)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_DbError(t *testing.T) {
	targetURL := "https://example.org/db"
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			return htmlResponse(req, http.StatusOK, `<html><body>Data</body></html>`), nil
		},
	}

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("", fmt.Errorf("db error"))

	// 4. Create crawler config
	cfg := newTestConfig(t, targetURL, transport)

	// 5. Create and run the crawler
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_QueueError(t *testing.T) {
	targetURL := "https://example.org/queue"
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			return htmlResponse(req, http.StatusOK, `<html><body>Data</body></html>`), nil
		},
	}

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(fmt.Errorf("queue error"))

	// 4. Create crawler config
	cfg := newTestConfig(t, targetURL, transport)

	// 5. Create and run the crawler
	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

func TestCollyCrawler_Run_FollowsLinks(t *testing.T) {
	startURL := "https://example.org/start"
	followURL := "https://example.org/follow"

	visitedFollowURL := false
	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == startURL {
				html := fmt.Sprintf(`<html><body><a href="%s">Follow me</a></body></html>`, followURL)
				return htmlResponse(req, http.StatusOK, html), nil
			}
			if req.URL.String() == followURL {
				visitedFollowURL = true
				return htmlResponse(req, http.StatusOK, `<html><body>You followed me!</body></html>`), nil
			}
			return htmlResponse(req, http.StatusNotFound, ``), nil
		},
	}

	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(nil)

	cfg := newTestConfig(t, startURL, transport)
	cfg.MaxDepth = 2 // Allow following links

	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	require.True(t, visitedFollowURL, "Expected crawler to follow the link")
}

func TestCollyCrawler_Run_FollowsLinks_VisitError(t *testing.T) {
	startURL := "https://example.org/start"
	followURL := "invalid-url"

	transport := mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == startURL {
				html := fmt.Sprintf(`<html><body><a href="%s">Follow me</a></body></html>`, followURL)
				return htmlResponse(req, http.StatusOK, html), nil
			}
			return htmlResponse(req, http.StatusNotFound, ``), nil
		},
	}

	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(nil)

	logger := zaptest.NewLogger(t)
	crawler := NewCollyCrawler(newTestConfig(t, startURL, transport), logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())
}

func TestCollyCrawler_HandleErrorBlocksDomain(t *testing.T) {
	cfg := Config{
		MaxForbiddenResponses: 1,
		RateLimitBackoff:      time.Nanosecond,
	}
	crawler := newInstrumentedCrawler(t, cfg)

	resp := &colly.Response{
		StatusCode: http.StatusForbidden,
		Request:    &colly.Request{URL: mustParseURL(t, "https://example.org/restricted")},
	}

	handler := crawler.handleError(context.Background())
	handler(resp, fmt.Errorf("forbidden"))

	require.False(t,
		crawler.shouldVisit("https://example.org/next"),
		"expected domain to be blocked after forbidden response",
	)
}

func TestCollyCrawler_ShouldVisitRespectsBlockedDomains(t *testing.T) {
	cfg := Config{
		BlockedDomains: []string{"example.org"},
	}
	crawler := newInstrumentedCrawler(t, cfg)

	require.False(t, crawler.shouldVisit("https://example.org/page"))
	require.True(t, crawler.shouldVisit("https://allowed.org/page"))
}

func TestCollyCrawler_ShouldVisitRespectsWildcardBlockedDomains(t *testing.T) {
	cfg := Config{
		BlockedDomains: []string{"*.ru"},
	}
	crawler := newInstrumentedCrawler(t, cfg)

	require.False(t, crawler.shouldVisit("https://example.ru"))
	require.False(t, crawler.shouldVisit("https://sub.example.ru"))
	require.True(t, crawler.shouldVisit("https://example.com"))
}

func TestCollyCrawler_HandleErrorRateLimitPauses(t *testing.T) {
	cfg := Config{
		RateLimitBackoff:      2 * time.Millisecond,
		MaxForbiddenResponses: 2,
	}
	crawler := newInstrumentedCrawler(t, cfg)
	spy := &spyPauser{}
	crawler.pauser = spy

	resp := &colly.Response{
		StatusCode: http.StatusTooManyRequests,
		Request:    &colly.Request{URL: mustParseURL(t, "https://example.org/limited")},
	}

	handler := crawler.handleError(context.Background())
	handler(resp, fmt.Errorf("rate limited"))

	require.Equal(t, 1, spy.calls)
	require.Equal(t, cfg.RateLimitBackoff, spy.delay)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return parsed
}

type spyPauser struct {
	calls int
	delay time.Duration
}

func (s *spyPauser) Pause(_ context.Context, delay time.Duration) {
	s.calls++
	s.delay = delay
}
