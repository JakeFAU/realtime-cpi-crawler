// Package headless contains fetchers that execute JavaScript via browsers.
package headless

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Config controls the behavior of the headless fetcher.
type Config struct {
	MaxParallel       int
	UserAgent         string
	NavigationTimeout time.Duration
}

// Fetcher implements crawler.Fetcher using chromedp and headless Chrome.
type Fetcher struct {
	cfg         Config
	limiter     chan struct{}
	allocator   context.Context
	allocCancel context.CancelFunc
}

// NewChromedp creates a headless fetcher backed by chromedp.
func NewChromedp(cfg Config) (*Fetcher, error) {
	if cfg.MaxParallel < 0 {
		return nil, fmt.Errorf("max parallel must be >= 0")
	}
	if cfg.NavigationTimeout <= 0 {
		cfg.NavigationTimeout = 45 * time.Second
	}
	var limiter chan struct{}
	if cfg.MaxParallel > 0 {
		limiter = make(chan struct{}, cfg.MaxParallel)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("enable-automation", false),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	return &Fetcher{
		cfg:         cfg,
		limiter:     limiter,
		allocator:   allocCtx,
		allocCancel: allocCancel,
	}, nil
}

// Close cancels the allocator context.
func (f *Fetcher) Close() {
	f.allocCancel()
}

// Fetch navigates with a headless browser and returns the fully rendered DOM.
func (f *Fetcher) Fetch(ctx context.Context, request crawler.FetchRequest) (crawler.FetchResponse, error) {
	if err := f.acquire(ctx); err != nil {
		return crawler.FetchResponse{}, err
	}
	defer f.release()

	taskCtx, taskCancel := chromedp.NewContext(f.allocator)
	defer taskCancel()

	taskCtx, cancel := context.WithTimeout(taskCtx, f.navTimeout())
	defer cancel()

	meta := newResponseMeta()
	chromedp.ListenTarget(taskCtx, meta.captureEvent)

	start := time.Now()
	html, finalURL, err := f.runHeadless(taskCtx, request)
	if err != nil {
		return crawler.FetchResponse{}, err
	}

	status, headers, responseURL := meta.snapshotWithFallbacks(request.URL, finalURL)
	if headers == nil {
		headers = http.Header{}
	}

	return crawler.FetchResponse{
		URL:          responseURL,
		StatusCode:   status,
		Headers:      headers,
		Body:         []byte(html),
		Duration:     time.Since(start),
		UsedHeadless: true,
	}, nil
}

func (f *Fetcher) runHeadless(ctx context.Context, request crawler.FetchRequest) (string, string, error) {
	var (
		html     string
		finalURL string
	)
	actions := []chromedp.Action{
		f.networkSetupAction(request.Headers),
		chromedp.Navigate(request.URL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500 * time.Millisecond),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	}
	if err := chromedp.Run(ctx, actions...); err != nil {
		return "", "", fmt.Errorf("chromedp run: %w", err)
	}
	return html, finalURL, nil
}

func (f *Fetcher) networkSetupAction(headers http.Header) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if err := network.Enable().Do(ctx); err != nil {
			return fmt.Errorf("enable network domain: %w", err)
		}
		if f.cfg.UserAgent != "" {
			if err := emulation.SetUserAgentOverride(f.cfg.UserAgent).Do(ctx); err != nil {
				return fmt.Errorf("set user-agent: %w", err)
			}
		}
		if len(headers) > 0 {
			if err := network.SetExtraHTTPHeaders(toNetworkHeaders(headers)).Do(ctx); err != nil {
				return fmt.Errorf("set extra headers: %w", err)
			}
		}
		return nil
	})
}

func (f *Fetcher) acquire(ctx context.Context) error {
	if f.limiter == nil {
		return nil
	}
	select {
	case f.limiter <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("headless slot wait canceled: %w", ctx.Err())
	}
}

func (f *Fetcher) release() {
	if f.limiter == nil {
		return
	}
	select {
	case <-f.limiter:
	default:
	}
}

type responseMeta struct {
	mu      sync.RWMutex
	status  int
	headers http.Header
	url     string
}

func newResponseMeta() *responseMeta {
	return &responseMeta{
		headers: http.Header{},
	}
}

func (m *responseMeta) capture(event *network.EventResponseReceived) {
	if event.Type != network.ResourceTypeDocument || event.Response == nil {
		return
	}
	headers := http.Header{}
	for key, value := range event.Response.Headers {
		switch v := value.(type) {
		case string:
			headers.Add(key, v)
		case []string:
			for _, entry := range v {
				headers.Add(key, entry)
			}
		case []interface{}:
			for _, entry := range v {
				headers.Add(key, fmt.Sprint(entry))
			}
		default:
			headers.Add(key, fmt.Sprint(v))
		}
	}
	m.mu.Lock()
	m.status = int(event.Response.Status)
	m.headers = headers
	m.url = event.Response.URL
	m.mu.Unlock()
}

func (m *responseMeta) snapshot() (int, http.Header, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, cloneHeader(m.headers), m.url
}

func (m *responseMeta) captureEvent(ev any) {
	if resp, ok := ev.(*network.EventResponseReceived); ok {
		m.capture(resp)
	}
}

func (m *responseMeta) snapshotWithFallbacks(requestURL, finalURL string) (int, http.Header, string) {
	status, headers, url := m.snapshot()
	switch {
	case url != "":
	case finalURL != "":
		url = finalURL
	default:
		url = requestURL
	}

	if status == 0 {
		status = http.StatusOK
	}
	return status, headers, url
}

func (f *Fetcher) navTimeout() time.Duration {
	if f.cfg.NavigationTimeout > 0 {
		return f.cfg.NavigationTimeout
	}
	return 45 * time.Second
}

func cloneHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	dst := make(http.Header, len(src))
	for k, values := range src {
		for _, v := range values {
			dst.Add(k, v)
		}
	}
	return dst
}

func toNetworkHeaders(h http.Header) network.Headers {
	headers := network.Headers{}
	for key, values := range h {
		if len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			headers[key] = values[0]
		} else {
			headers[key] = append([]string(nil), values...)
		}
	}
	return headers
}
