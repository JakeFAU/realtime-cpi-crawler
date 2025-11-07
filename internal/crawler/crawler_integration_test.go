package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestEngineIntegration(t *testing.T) {
	tmp := t.TempDir()
	cfg := CrawlerConfig{
		Seeds:                  []string{"https://example.test/static"},
		UserAgent:              "TestAgent",
		RespectRobots:          false,
		MaxDepth:               2,
		Concurrency:            2,
		RateLimitPerDomain:     10,
		RequestTimeout:         time.Second,
		JSRenderTimeout:        time.Second,
		JSRenderMaxConcurrency: 1,
		JSRenderDomainQPS:      10,
		EscalateOnlySameHost:   true,
		FeatureRenderEnabled:   true,
		DetectorMinHTMLBytes:   5,
		DetectorSelectorMust:   []string{"#content"},
		DetectorKeywords:       []string{"data-reactroot"},
		OutputDir:              tmp,
		MaxPageBytes:           1 << 20,
	}

	staticHTML := `<html><body><div id="content">static</div><a href="https://example.test/js"></a></body></html>`
	jsFast := `<html><body><div id="app" data-reactroot></div></body></html>`

	fetcher := &mapFetcher{
		data: map[string]Page{
			"https://example.test/static": {
				URL:        "https://example.test/static",
				FinalURL:   "https://example.test/static",
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/html"}},
				Body:       []byte(staticHTML),
			},
			"https://example.test/js": {
				URL:        "https://example.test/js",
				FinalURL:   "https://example.test/js",
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/html"}},
				Body:       []byte(jsFast),
			},
			"https://example.test/js-child": {
				URL:        "https://example.test/js-child",
				FinalURL:   "https://example.test/js-child",
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/html"}},
				Body:       []byte(`<html><body><div id="content">child</div></body></html>`),
			},
		},
	}

	renderer := &stubRenderer{
		pages: map[string]Page{
			"https://example.test/js": {
				URL:        "https://example.test/js",
				FinalURL:   "https://example.test/js",
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/html"}},
				Body:       []byte(`<html><body><div id="content">rendered</div><a href="https://example.test/js-child"></a></body></html>`),
				UsedJS:     true,
			},
		},
	}

	logger := zap.NewNop()
	detector := NewHeuristicDetector(cfg.DetectorMinHTMLBytes, cfg.DetectorSelectorMust, cfg.DetectorKeywords)
	sink, err := NewFileSystemSink(cfg.OutputDir, cfg.MaxPageBytes, logger)
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	engine := NewEngine(cfg, fetcher, renderer, detector, sink, NewRobotsEnforcer(false, cfg.UserAgent, logger), NewExponentialRetryPolicy(), logger)
	if err := engine.Run(context.Background()); err != nil {
		t.Fatalf("engine run: %v", err)
	}

	metas := readMetadata(t, tmp)
	if len(metas) != 3 {
		t.Fatalf("expected 3 metadata records, got %d", len(metas))
	}
	jsMeta := metas["https://example.test/js"]
	if !jsMeta.UsedJS {
		t.Fatal("expected JS page metadata to mark used_js")
	}
	if metas["https://example.test/static"].UsedJS {
		t.Fatal("static page should not use JS")
	}
	if _, ok := metas["https://example.test/js-child"]; !ok {
		t.Fatal("child page discovered via rendering should be crawled")
	}
}

func TestEngineRenderTimeout(t *testing.T) {
	tmp := t.TempDir()
	cfg := CrawlerConfig{
		Seeds:                  []string{"https://slow.test/js"},
		UserAgent:              "TestAgent",
		RespectRobots:          false,
		MaxDepth:               0,
		Concurrency:            1,
		RateLimitPerDomain:     1,
		RequestTimeout:         time.Second,
		JSRenderTimeout:        50 * time.Millisecond,
		JSRenderMaxConcurrency: 1,
		JSRenderDomainQPS:      1,
		EscalateOnlySameHost:   true,
		FeatureRenderEnabled:   true,
		DetectorMinHTMLBytes:   100,
		DetectorSelectorMust:   []string{"#content"},
		DetectorKeywords:       []string{"data-reactroot"},
		OutputDir:              tmp,
		MaxPageBytes:           1 << 20,
	}

	fetcher := &mapFetcher{
		data: map[string]Page{
			"https://slow.test/js": {
				URL:        "https://slow.test/js",
				FinalURL:   "https://slow.test/js",
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"text/html"}},
				Body:       []byte(`<html><body><div data-reactroot id="app"></div></body></html>`),
			},
		},
	}

	renderer := &slowRenderer{delay: time.Second}
	logger := zap.NewNop()
	detector := NewHeuristicDetector(cfg.DetectorMinHTMLBytes, cfg.DetectorSelectorMust, cfg.DetectorKeywords)
	sink, err := NewFileSystemSink(cfg.OutputDir, cfg.MaxPageBytes, logger)
	if err != nil {
		t.Fatalf("sink: %v", err)
	}
	engine := NewEngine(cfg, fetcher, renderer, detector, sink, NewRobotsEnforcer(false, cfg.UserAgent, logger), NewExponentialRetryPolicy(), logger)
	if err := engine.Run(context.Background()); err != nil {
		t.Fatalf("engine run: %v", err)
	}

	metas := readMetadata(t, tmp)
	if len(metas) != 1 {
		t.Fatalf("expected 1 metadata record, got %d", len(metas))
	}
	if metas["https://slow.test/js"].UsedJS {
		t.Fatal("render timeout should not mark used_js")
	}
}

type mapFetcher struct {
	data map[string]Page
}

func (m *mapFetcher) Fetch(_ context.Context, rawURL string) (Page, error) {
	p, ok := m.data[rawURL]
	if !ok {
		return Page{}, errors.New("missing page")
	}
	return p, nil
}

type stubRenderer struct {
	pages map[string]Page
}

func (s *stubRenderer) Render(_ context.Context, rawURL string) (Page, error) {
	p, ok := s.pages[rawURL]
	if !ok {
		return Page{}, errors.New("render miss")
	}
	return p, nil
}

func (s *stubRenderer) Close(context.Context) error { return nil }

type slowRenderer struct {
	delay time.Duration
}

func (s *slowRenderer) Render(ctx context.Context, rawURL string) (Page, error) {
	select {
	case <-time.After(s.delay):
		return Page{
			URL:        rawURL,
			FinalURL:   rawURL,
			StatusCode: http.StatusOK,
			Headers:    http.Header{"Content-Type": []string{"text/html"}},
			Body:       []byte(`<html><body><div id="content">slow</div></body></html>`),
			UsedJS:     true,
		}, nil
	case <-ctx.Done():
		return Page{}, ctx.Err()
	}
}

func (s *slowRenderer) Close(context.Context) error { return nil }

func readMetadata(t *testing.T, root string) map[string]CrawlMetadata {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	out := make(map[string]CrawlMetadata)
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read meta: %v", err)
		}
		var meta CrawlMetadata
		if err := json.Unmarshal(content, &meta); err != nil {
			t.Fatalf("unmarshal meta: %v", err)
		}
		out[meta.URL] = meta
	}
	return out
}
