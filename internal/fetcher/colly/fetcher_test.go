package collyfetcher

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gocolly/colly/v2"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestFetcherBuildCollector(t *testing.T) {
	t.Parallel()

	f := New(Config{UserAgent: "coverage-agent", RespectRobots: true, Timeout: time.Second})
	start := time.Unix(0, 0)
	req := crawler.FetchRequest{
		URL:                   "https://example.com",
		Headers:               http.Header{"X-Trace": {"yes"}},
		RespectRobotsProvided: true,
		RespectRobots:         false,
	}

	collector := f.buildCollector(req, start, &crawler.FetchResponse{}, new(error))
	if collector.UserAgent != "coverage-agent" {
		t.Fatalf("expected user agent override, got %q", collector.UserAgent)
	}
	if !collector.IgnoreRobotsTxt {
		t.Fatal("expected robots txt to be ignored when request overrides")
	}
}

func TestConfigureCollectorHooks(t *testing.T) {
	t.Parallel()

	f := New(Config{})
	req := crawler.FetchRequest{
		URL:     "https://example.com",
		Headers: http.Header{"X-Trace": {"yes"}},
	}
	start := time.Unix(0, 0)
	var result crawler.FetchResponse
	var fetchErr error

	hooks := &stubHooks{}
	f.configureCollectorHooks(hooks, req, start, &result, &fetchErr)
	if hooks.onRequest == nil || hooks.onResponse == nil || hooks.onError == nil {
		t.Fatal("expected hooks to be registered")
	}

	collyReq := &colly.Request{Headers: &http.Header{}}
	hooks.onRequest(collyReq)
	if collyReq.Headers.Get("X-Trace") != "yes" {
		t.Fatalf("expected header propagation, got %+v", collyReq.Headers)
	}

	hooks.onResponse(&colly.Response{
		StatusCode: http.StatusCreated,
		Body:       []byte("body"),
		Headers:    &http.Header{"X-Resp": {"ok"}},
		Request: &colly.Request{
			URL: mustParseURL(t, "https://example.com"),
		},
	})
	if result.StatusCode != http.StatusCreated || string(result.Body) != "body" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Headers.Get("X-Resp") != "ok" {
		t.Fatalf("expected headers copied, got %+v", result.Headers)
	}

	hooks.onError(nil, errors.New("boom"))
	if fetchErr == nil || fetchErr.Error() != "boom" {
		t.Fatalf("expected fetchErr set, got %v", fetchErr)
	}
}

func TestCopyHeadersHandlesNil(t *testing.T) {
	t.Parallel()

	f := New(Config{})
	collyReq := &colly.Request{Headers: &http.Header{}}
	f.copyHeaders(crawler.FetchRequest{}, collyReq)
	if len(*collyReq.Headers) != 0 {
		t.Fatalf("expected no headers to be copied, got %+v", *collyReq.Headers)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse url %q: %v", raw, err)
	}
	return u
}

type stubHooks struct {
	onRequest  colly.RequestCallback
	onResponse colly.ResponseCallback
	onError    colly.ErrorCallback
}

func (s *stubHooks) OnRequest(cb colly.RequestCallback) {
	s.onRequest = cb
}

func (s *stubHooks) OnResponse(cb colly.ResponseCallback) {
	s.onResponse = cb
}

func (s *stubHooks) OnError(cb colly.ErrorCallback) {
	s.onError = cb
}
