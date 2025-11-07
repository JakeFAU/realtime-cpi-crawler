package crawler

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gocolly/colly/v2"
	"go.uber.org/zap"
)

// CollyFetcher implements the Fetcher interface using the Colly collector.
type CollyFetcher struct {
	baseCollector *colly.Collector
	logger        *zap.Logger
}

// NewCollyFetcher constructs a configured Colly-based Fetcher.
func NewCollyFetcher(cfg CrawlerConfig, logger *zap.Logger) (*CollyFetcher, error) {
	opts := []colly.CollectorOption{
		colly.Async(true),
		colly.UserAgent(cfg.UserAgent),
	}
	base := colly.NewCollector(opts...)
	base.AllowURLRevisit = false
	base.WithTransport(&http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       cfg.Concurrency * 2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		ForceAttemptHTTP2:     true,
	})
	base.SetRequestTimeout(cfg.RequestTimeout)

	delay := time.Second / time.Duration(max(1, cfg.RateLimitPerDomain))
	if err := base.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.Concurrency,
		Delay:       delay,
	}); err != nil {
		return nil, err
	}

	return &CollyFetcher{
		baseCollector: base,
		logger:        logger,
	}, nil
}

// Fetch retrieves a page via the configured Colly collector.
func (f *CollyFetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	collector := f.baseCollector.Clone()
	resultCh := make(chan fetchResult, 1)
	var once sync.Once
	send := func(res fetchResult) {
		once.Do(func() {
			resultCh <- res
		})
	}

	collector.OnResponse(func(r *colly.Response) {
		headers := http.Header{}
		if r.Headers != nil {
			for k, v := range *r.Headers {
				cp := make([]string, len(v))
				copy(cp, v)
				headers[k] = cp
			}
		}
		page := Page{
			URL:        rawURL,
			FinalURL:   r.Request.URL.String(),
			StatusCode: r.StatusCode,
			Headers:    headers,
			Body:       append([]byte{}, r.Body...),
		}
		send(fetchResult{page: page})
	})

	collector.OnError(func(_ *colly.Response, err error) {
		if err == nil {
			err = errors.New("unknown colly error")
		}
		send(fetchResult{err: err})
	})

	if err := collector.Visit(rawURL); err != nil {
		return Page{}, err
	}
	collector.Wait()

	select {
	case res := <-resultCh:
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		return res.page, res.err
	default:
		return Page{}, errors.New("colly fetch produced no result")
	}
}

type fetchResult struct {
	page Page
	err  error
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
