package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ErrRendererDisabled indicates rendering has been disabled via configuration.
var ErrRendererDisabled = errors.New("renderer disabled")

// ChromedpRenderer renders pages using headless Chrome via chromedp.
type ChromedpRenderer struct {
	allocatorCancel context.CancelFunc
	browserCtx      context.Context
	browserCancel   context.CancelFunc
	logger          *zap.Logger
	sem             chan struct{}
	timeout         time.Duration
	domainQPS       float64
	domainLimiters  sync.Map
	userAgent       string
}

// NewChromedpRenderer creates a renderer using the provided configuration.
func NewChromedpRenderer(cfg CrawlerConfig, logger *zap.Logger) (*ChromedpRenderer, error) {
	if cfg.JSRenderMaxConcurrency <= 0 {
		return nil, ErrRendererDisabled
	}

	opts := chromedp.DefaultExecAllocatorOptions[:]
	opts = append(opts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.UserAgent(cfg.UserAgent),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		allocatorCancel()
		browserCancel()
		return nil, fmt.Errorf("chromedp warmup: %w", err)
	}

	return &ChromedpRenderer{
		allocatorCancel: allocatorCancel,
		browserCtx:      browserCtx,
		browserCancel:   browserCancel,
		logger:          logger,
		sem:             make(chan struct{}, cfg.JSRenderMaxConcurrency),
		timeout:         cfg.JSRenderTimeout,
		domainQPS:       cfg.JSRenderDomainQPS,
		userAgent:       cfg.UserAgent,
	}, nil
}

// Close tears down the chromedp allocator and browser contexts.
func (r *ChromedpRenderer) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.browserCancel()
	r.allocatorCancel()
	select {
	case <-ctx.Done():
	default:
	}
	return nil
}

// Render executes the page with JavaScript enabled and returns the DOM snapshot.
func (r *ChromedpRenderer) Render(ctx context.Context, rawURL string) (Page, error) {
	if r == nil {
		return Page{}, ErrRendererDisabled
	}

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return Page{}, ctx.Err()
	}

	if err := r.waitDomainBudget(ctx, rawURL); err != nil {
		return Page{}, err
	}

	tabCtx, cancelTab := chromedp.NewContext(r.browserCtx)
	defer cancelTab()

	taskCtx, cancelTask := context.WithTimeout(tabCtx, r.timeout)
	defer cancelTask()

	if ctx.Done() != nil {
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				cancelTask()
			case <-done:
			}
		}()
		defer close(done)
	}

	var html string
	var finalURL string
	var statusCode int
	headers := http.Header{}
	var once sync.Once

	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		resp, ok := ev.(*network.EventResponseReceived)
		if !ok {
			return
		}
		if resp.Type != network.ResourceTypeDocument {
			return
		}
		once.Do(func() {
			statusCode = int(resp.Response.Status)
			finalURL = resp.Response.URL
			for k, v := range resp.Response.Headers {
				headers.Add(k, fmt.Sprint(v))
			}
		})
	})

	tasks := chromedp.Tasks{
		network.Enable(),
		emulation.SetUserAgentOverride(r.userAgent),
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	}

	if err := chromedp.Run(taskCtx, tasks); err != nil {
		return Page{}, err
	}
	if finalURL == "" {
		finalURL = rawURL
	}

	return Page{
		URL:        rawURL,
		FinalURL:   finalURL,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       []byte(html),
		UsedJS:     true,
	}, nil
}

func (r *ChromedpRenderer) waitDomainBudget(ctx context.Context, rawURL string) error {
	if r.domainQPS <= 0 {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(parsed.Host)
	val, _ := r.domainLimiters.LoadOrStore(host, rate.NewLimiter(rate.Limit(r.domainQPS), 1))
	lim := val.(*rate.Limiter)
	return lim.Wait(ctx)
}
