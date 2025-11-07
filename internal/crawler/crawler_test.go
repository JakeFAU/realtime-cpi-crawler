package crawler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockFetcher is a mock implementation of the Fetcher interface.
type MockFetcher struct {
	mock.Mock
}

func (m *MockFetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	args := m.Called(ctx, rawURL)
	return args.Get(0).(Page), args.Error(1)
}

// MockRenderer is a mock implementation of the Renderer interface.
type MockRenderer struct {
	mock.Mock
}

func (m *MockRenderer) Render(ctx context.Context, rawURL string) (Page, error) {
	args := m.Called(ctx, rawURL)
	return args.Get(0).(Page), args.Error(1)
}

func (m *MockRenderer) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockDetector is a mock implementation of the Detector interface.
type MockDetector struct {
	mock.Mock
}

func (m *MockDetector) NeedsJS(ctx context.Context, page Page) bool {
	args := m.Called(ctx, page)
	return args.Bool(0)
}

// MockRobotsPolicy is a mock implementation of the RobotsPolicy interface.
type MockRobotsPolicy struct {
	mock.Mock
}

func (m *MockRobotsPolicy) Allowed(ctx context.Context, rawURL string) bool {
	args := m.Called(ctx, rawURL)
	return args.Bool(0)
}

// MockRetryPolicy is a mock implementation of the RetryPolicy interface.
type MockRetryPolicy struct {
	mock.Mock
}

func (m *MockRetryPolicy) ShouldRetry(err error, attempt int) bool {
	args := m.Called(err, attempt)
	return args.Bool(0)
}

func (m *MockRetryPolicy) Backoff(attempt int) time.Duration {
	args := m.Called(attempt)
	return args.Get(0).(time.Duration)
}

// MockStorageSink is a mock implementation of the StorageSink interface.
type MockStorageSink struct {
	mock.Mock
}

func (m *MockStorageSink) SaveHTML(ctx context.Context, page Page) (string, error) {
	args := m.Called(ctx, page)
	return args.String(0), args.Error(1)
}

func (m *MockStorageSink) SaveMeta(ctx context.Context, meta CrawlMetadata) error {
	args := m.Called(ctx, meta)
	return args.Error(0)
}

func TestEngine_Run(t *testing.T) {
	t.Run("simple crawl", func(t *testing.T) {
		// Arrange
		cfg := Config{
			Seeds:       []string{"http://example.com"},
			Concurrency: 1,
			MaxDepth:    0,
		}
		fetcher := new(MockFetcher)
		renderer := new(MockRenderer)
		detector := new(MockDetector)
		sink := new(MockStorageSink)
		robots := new(MockRobotsPolicy)
		retry := new(MockRetryPolicy)
		engine := NewEngine(cfg, fetcher, renderer, detector, sink, robots, retry, nil)

		page := Page{
			URL:        "http://example.com/",
			FinalURL:   "http://example.com/",
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"text/html"}},
			Body:       []byte("<html></html>"),
		}

		robots.On("Allowed", mock.Anything, "http://example.com/").Return(true)
		fetcher.On("Fetch", mock.Anything, "http://example.com/").Return(page, nil)
		detector.On("NeedsJS", mock.Anything, page).Return(false)
		sink.On("SaveHTML", mock.Anything, page).Return("path/to/page.html", nil)
		sink.On("SaveMeta", mock.Anything, mock.Anything).Return(nil)

		// Act
		err := engine.Run(context.Background())

		// Assert
		require.NoError(t, err)
		fetcher.AssertExpectations(t)
		sink.AssertExpectations(t)
	})

	t.Run("follows links", func(t *testing.T) {
		// Arrange
		cfg := Config{
			Seeds:       []string{"http://example.com"},
			Concurrency: 1,
			MaxDepth:    1,
		}
		fetcher := new(MockFetcher)
		renderer := new(MockRenderer)
		detector := new(MockDetector)
		sink := new(MockStorageSink)
		robots := new(MockRobotsPolicy)
		retry := new(MockRetryPolicy)
		engine := NewEngine(cfg, fetcher, renderer, detector, sink, robots, retry, nil)

		page1 := Page{
			URL:        "http://example.com/",
			FinalURL:   "http://example.com/",
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"text/html"}},
			Body:       []byte("<html><a href=\"http://example.com/page2\">Page 2</a></html>"),
		}
		page2 := Page{
			URL:        "http://example.com/page2",
			FinalURL:   "http://example.com/page2",
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"text/html"}},
			Body:       []byte("<html></html>"),
		}

		robots.On("Allowed", mock.Anything, mock.Anything).Return(true)
		fetcher.On("Fetch", mock.Anything, "http://example.com/").Return(page1, nil)
		fetcher.On("Fetch", mock.Anything, "http://example.com/page2").Return(page2, nil)
		detector.On("NeedsJS", mock.Anything, mock.Anything).Return(false)
		sink.On("SaveHTML", mock.Anything, mock.Anything).Return("path", nil)
		sink.On("SaveMeta", mock.Anything, mock.Anything).Return(nil)

		// Act
		err := engine.Run(context.Background())

		// Assert
		require.NoError(t, err)
		fetcher.AssertCalled(t, "Fetch", mock.Anything, "http://example.com/")
		fetcher.AssertCalled(t, "Fetch", mock.Anything, "http://example.com/page2")
	})

	t.Run("handles malformed seed", func(t *testing.T) {
		cfg := Config{Seeds: []string{":"}}
		engine := NewEngine(cfg, nil, nil, nil, nil, nil, nil, nil)
		err := engine.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("respects robots.txt", func(t *testing.T) {
		cfg := Config{
			Seeds:       []string{"http://example.com"},
			Concurrency: 1,
		}
		fetcher := new(MockFetcher)
		robots := new(MockRobotsPolicy)
		engine := NewEngine(cfg, fetcher, nil, nil, nil, robots, nil, nil)

		robots.On("Allowed", mock.Anything, "http://example.com/").Return(false)

		err := engine.Run(context.Background())

		require.NoError(t, err)
		fetcher.AssertNotCalled(t, "Fetch", mock.Anything, mock.Anything)
	})

	t.Run("handles fetch error", func(t *testing.T) {
		cfg := Config{
			Seeds:       []string{"http://example.com"},
			Concurrency: 1,
		}
		fetcher := new(MockFetcher)
		robots := new(MockRobotsPolicy)
		retry := new(MockRetryPolicy)
		engine := NewEngine(cfg, fetcher, nil, nil, nil, robots, retry, nil)

		robots.On("Allowed", mock.Anything, "http://example.com/").Return(true)
		fetcher.On("Fetch", mock.Anything, "http://example.com/").Return(Page{}, errors.New("fetch error"))
		retry.On("ShouldRetry", mock.Anything, mock.Anything).Return(false)

		err := engine.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("context cancelled", func(t *testing.T) {
		cfg := Config{
			Seeds:       []string{"http://example.com"},
			Concurrency: 1,
		}
		engine := NewEngine(cfg, new(MockFetcher), nil, nil, nil, new(MockRobotsPolicy), nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := engine.Run(ctx)
		require.Error(t, err)
	})
}

func Test_extractLinks(t *testing.T) {
	t.Run("no links", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com",
			Body:     []byte("<html><body><h1>No links here</h1></body></html>"),
		}
		links := extractLinks(page)
		require.Empty(t, links)
	})

	t.Run("absolute links", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com",
			Body:     []byte("<html><body><a href=\"http://example.com/page2\">Page 2</a><a href=\"http://another.com/page3\">Page 3</a></body></html>"),
		}
		links := extractLinks(page)
		require.ElementsMatch(t, []string{"http://example.com/page2", "http://another.com/page3"}, links)
	})

	t.Run("relative links", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com/path/",
			Body:     []byte("<html><body><a href=\"/page2\">Page 2</a><a href=\"page3\">Page 3</a></body></html>"),
		}
		links := extractLinks(page)
		require.ElementsMatch(t, []string{"http://example.com/page2", "http://example.com/path/page3"}, links)
	})

	t.Run("mixed links", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com",
			Body:     []byte("<html><body><a href=\"http://example.com/page2\">Page 2</a><a href=\"/page3\">Page 3</a></body></html>"),
		}
		links := extractLinks(page)
		require.ElementsMatch(t, []string{"http://example.com/page2", "http://example.com/page3"}, links)
	})

	t.Run("malformed links", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com",
			Body:     []byte("<html><body><a href=\":\">Malformed</a></body></html>"),
		}
		links := extractLinks(page)
		require.Empty(t, links)
	})

	t.Run("empty body", func(t *testing.T) {
		page := Page{
			FinalURL: "http://example.com",
			Body:     []byte(""),
		}
		links := extractLinks(page)
		require.Empty(t, links)
	})
}
