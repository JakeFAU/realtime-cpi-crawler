// Package crawler provides the implementation of the web crawler using the Colly library.
package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestCollyCrawler_Run(t *testing.T) {
	// 1. Setup a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><a href="/">Home</a><p>$123.45</p></body></html>`))
	}))
	defer server.Close()

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(nil)

	// 4. Create crawler config
	cfg := Config{
		URLFilters: []*regexp.Regexp{
			regexp.MustCompile(regexp.QuoteMeta(server.URL) + ".*"),
		},
		UserAgent:         "test-agent",
		HTTPTimeout:       10 * time.Second,
		MaxDepth:          1,
		InitialTargetURLs: []string{server.URL},
		Concurrency:       1,
	}

	// 5. Create and run the crawler
	logger, _ := zap.NewDevelopment()
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}

func TestCollyCrawler_Run_HttpError(t *testing.T) {
	// 1. Setup a test HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Create crawler config
	cfg := Config{
		URLFilters: []*regexp.Regexp{
			regexp.MustCompile(regexp.QuoteMeta(server.URL) + ".*"),
		},
		UserAgent:         "test-agent",
		HTTPTimeout:       10 * time.Second,
		MaxDepth:          1,
		InitialTargetURLs: []string{server.URL},
		Concurrency:       1,
	}

	// 4. Create and run the crawler
	logger, _ := zap.NewDevelopment()
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 5. Assert that the mocks were not called
	mockStorage.AssertNotCalled(t, "Save", mock.Anything, mock.Anything, mock.Anything)
	mockDB.AssertNotCalled(t, "SaveCrawl", mock.Anything, mock.Anything)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_StorageError(t *testing.T) {
	// 1. Setup a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><a href="/">Home</a><p>$123.45</p></body></html>`))
	}))
	defer server.Close()

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(fmt.Errorf("storage error"))

	// 4. Create crawler config
	cfg := Config{
		URLFilters: []*regexp.Regexp{
			regexp.MustCompile(regexp.QuoteMeta(server.URL) + ".*"),
		},
		UserAgent:         "test-agent",
		HTTPTimeout:       10 * time.Second,
		MaxDepth:          1,
		InitialTargetURLs: []string{server.URL},
		Concurrency:       1,
	}

	// 5. Create and run the crawler
	logger, _ := zap.NewDevelopment()
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertNotCalled(t, "SaveCrawl", mock.Anything, mock.Anything)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_DbError(t *testing.T) {
	// 1. Setup a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><a href="/">Home</a><p>$123.45</p></body></html>`))
	}))
	defer server.Close()

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("", fmt.Errorf("db error"))

	// 4. Create crawler config
	cfg := Config{
		URLFilters: []*regexp.Regexp{
			regexp.MustCompile(regexp.QuoteMeta(server.URL) + ".*"),
		},
		UserAgent:         "test-agent",
		HTTPTimeout:       10 * time.Second,
		MaxDepth:          1,
		InitialTargetURLs: []string{server.URL},
		Concurrency:       1,
	}

	// 5. Create and run the crawler
	logger, _ := zap.NewDevelopment()
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

func TestCollyCrawler_Run_QueueError(t *testing.T) {
	// 1. Setup a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><a href="/">Home</a><p>$123.45</p></body></html>`))
	}))
	defer server.Close()

	// 2. Create mock providers
	mockStorage := new(storage.MockProvider)
	mockDB := new(database.MockProvider)
	mockQueue := new(queue.MockProvider)

	// 3. Setup expectations
	mockStorage.On("Save", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("[]uint8")).Return(nil)
	mockDB.On("SaveCrawl", mock.Anything, mock.AnythingOfType("database.CrawlMetadata")).Return("test-crawl-id", nil)
	mockQueue.On("Publish", mock.Anything, "test-crawl-id").Return(fmt.Errorf("queue error"))

	// 4. Create crawler config
	cfg := Config{
		URLFilters: []*regexp.Regexp{
			regexp.MustCompile(regexp.QuoteMeta(server.URL) + ".*"),
		},
		UserAgent:         "test-agent",
		HTTPTimeout:       10 * time.Second,
		MaxDepth:          1,
		InitialTargetURLs: []string{server.URL},
		Concurrency:       1,
	}

	// 5. Create and run the crawler
	logger, _ := zap.NewDevelopment()
	crawler := NewCollyCrawler(cfg, logger, mockStorage, mockDB, mockQueue)
	crawler.Run(context.Background())

	// 6. Assert that the mocks were called
	mockStorage.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockQueue.AssertExpectations(t)
}
