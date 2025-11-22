// Package collyfetcher implements Fetcher using gocolly.
package collyfetcher

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gocolly/colly/v2"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Config controls collector behavior.
type Config struct {
	UserAgent     string
	RespectRobots bool
	Timeout       time.Duration
}

// Fetcher implements crawler.Fetcher using the Colly collector.
type Fetcher struct {
	cfg           Config
	transport     http.RoundTripper
	baseCollector *colly.Collector
}

type collectorHooks interface {
	OnRequest(colly.RequestCallback)
	OnResponse(colly.ResponseCallback)
	OnError(colly.ErrorCallback)
}

// New builds a Fetcher.
func New(cfg Config) *Fetcher {
	c := colly.NewCollector(colly.Async(false))

	// Create a base transport with connection pooling
	baseTransport := newHTTPTransport()

	// Wrap with robots cache
	transport := NewRobotsCacheTransport(baseTransport)

	c.WithTransport(transport)

	return &Fetcher{
		cfg:           cfg,
		transport:     transport,
		baseCollector: c,
	}
}

// Fetch executes a single HTTP GET using Colly.
func (f *Fetcher) Fetch(ctx context.Context, request crawler.FetchRequest) (crawler.FetchResponse, error) {
	var (
		result   crawler.FetchResponse
		fetchErr error
	)
	start := time.Now()
	collector, robotsState := f.buildCollector(request, start, &result, &fetchErr)

	if err := f.runCollector(ctx, collector, request.URL, &fetchErr); err != nil {
		return crawler.FetchResponse{}, err
	}
	if robotsState != nil {
		robotsState.apply(&result)
	}
	return result, nil
}

func (f *Fetcher) buildCollector(
	request crawler.FetchRequest,
	start time.Time,
	result *crawler.FetchResponse,
	fetchErr *error,
) (*colly.Collector, *robotsProbeState) {
	collector := f.baseCollector.Clone()
	if f.cfg.UserAgent != "" {
		collector.UserAgent = f.cfg.UserAgent
	}
	respectRobots := f.cfg.RespectRobots
	if request.RespectRobotsProvided {
		respectRobots = request.RespectRobots
	}
	collector.IgnoreRobotsTxt = !respectRobots
	timeout := f.cfg.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	collector.SetRequestTimeout(timeout)

	var robotsState *robotsProbeState
	baseTransport := f.transport
	if baseTransport == nil {
		baseTransport = newHTTPTransport()
	}
	if respectRobots {
		robotsState = newRobotsProbeState()
		collector.WithTransport(&robotsAwareTransport{
			base:  baseTransport,
			state: robotsState,
		})
	} else {
		collector.WithTransport(baseTransport)
	}

	// just to make sure it's set
	result.JobID = request.JobID
	result.JobStartedAt = request.JobStartedAt

	f.configureCollectorHooks(collector, request, start, result, fetchErr)
	return collector, robotsState
}

func (f *Fetcher) configureCollectorHooks(
	hooks collectorHooks,
	request crawler.FetchRequest,
	start time.Time,
	result *crawler.FetchResponse,
	fetchErr *error,
) {
	hooks.OnRequest(func(r *colly.Request) {
		f.copyHeaders(request, r)
	})

	hooks.OnResponse(func(r *colly.Response) {
		*result = crawler.FetchResponse{
			URL:          r.Request.URL.String(),
			StatusCode:   r.StatusCode,
			Headers:      r.Headers.Clone(),
			Body:         append([]byte(nil), r.Body...),
			Duration:     time.Since(start),
			UsedHeadless: false,
			JobID:        result.JobID,
			JobStartedAt: result.JobStartedAt,
		}
	})

	hooks.OnError(func(_ *colly.Response, err error) {
		*fetchErr = err
	})
}

func (f *Fetcher) runCollector(ctx context.Context, collector *colly.Collector, url string, fetchErr *error) error {
	done := make(chan error, 1)
	go func() {
		done <- collector.Visit(url)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("colly fetch canceled: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return fmt.Errorf("colly visit failed: %w", err)
		}
		if *fetchErr != nil {
			return fmt.Errorf("colly response failed: %w", *fetchErr)
		}
		return nil
	}
}

func (f *Fetcher) copyHeaders(request crawler.FetchRequest, r *colly.Request) {
	if request.Headers == nil {
		return
	}
	for key, values := range request.Headers {
		for _, v := range values {
			r.Headers.Add(key, v)
		}
	}
}

func newHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}
}
