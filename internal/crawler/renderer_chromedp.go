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
func NewChromedpRenderer(cfg Config, logger *zap.Logger) (*ChromedpRenderer, error) {
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

	release, err := r.acquireSlot(ctx)
	if err != nil {
		return Page{}, err
	}
	defer release()

	if waitErr := r.waitDomainBudget(ctx, rawURL); waitErr != nil {
		return Page{}, fmt.Errorf("render rate limit: %w", waitErr)
	}

	tabCtx, cancelTab := chromedp.NewContext(r.browserCtx)
	defer cancelTab()

	taskCtx, cancelTask := context.WithTimeout(tabCtx, r.timeout)
	defer cancelTask()

	stopForward := forwardCancel(ctx, cancelTask)
	defer stopForward()

	meta := newResponseMeta()
	r.recordResponse(tabCtx, meta)

	html, err := r.runChromedp(taskCtx, rawURL)
	if err != nil {
		return Page{}, fmt.Errorf("chromedp run: %w", err)
	}

	return Page{
		URL:        rawURL,
		FinalURL:   meta.finalURL(rawURL),
		StatusCode: meta.statusCode,
		Headers:    meta.headers,
		Body:       []byte(html),
		UsedJS:     true,
	}, nil
}

func (r *ChromedpRenderer) acquireSlot(ctx context.Context) (func(), error) {
	if r.sem == nil {
		return func() {}, nil
	}
	select {
	case r.sem <- struct{}{}:
		return func() { <-r.sem }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("acquire render slot: %w", ctx.Err())
	}
}

type responseMeta struct {
	once       sync.Once
	statusCode int
	headers    http.Header
	url        string
}

func newResponseMeta() *responseMeta {
	return &responseMeta{
		headers: make(http.Header),
	}
}

func (m *responseMeta) finalURL(raw string) string {
	if m.url == "" {
		return raw
	}
	return m.url
}

func (r *ChromedpRenderer) recordResponse(tabCtx context.Context, meta *responseMeta) {
	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		resp, ok := ev.(*network.EventResponseReceived)
		if !ok || resp.Type != network.ResourceTypeDocument {
			return
		}
		meta.once.Do(func() {
			meta.statusCode = int(resp.Response.Status)
			meta.url = resp.Response.URL
			for k, v := range resp.Response.Headers {
				meta.headers.Add(k, fmt.Sprint(v))
			}
		})
	})
}

func (r *ChromedpRenderer) runChromedp(ctx context.Context, rawURL string) (string, error) {
	var html string
	tasks := chromedp.Tasks{
		network.Enable(),
		emulation.SetUserAgentOverride(r.userAgent),
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		return "", fmt.Errorf("chromedp run: %w", err)
	}
	return html, nil
}

func forwardCancel(parent context.Context, cancel context.CancelFunc) func() {
	if parent == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-parent.Done():
			cancel()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func (r *ChromedpRenderer) waitDomainBudget(ctx context.Context, rawURL string) error {
	if r.domainQPS <= 0 {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse render url: %w", err)
	}
	host := strings.ToLower(parsed.Host)
	val, _ := r.domainLimiters.LoadOrStore(host, rate.NewLimiter(rate.Limit(r.domainQPS), 1))
	limiter, ok := val.(*rate.Limiter)
	if !ok {
		return fmt.Errorf("unexpected limiter type %T", val)
	}
	if err := limiter.Wait(ctx); err != nil {
		return fmt.Errorf("wait limiter: %w", err)
	}
	return nil
}
