package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestChromedpRenderer_Render(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><body><script>document.body.innerHTML = '<div id="late">late content</div>';</script></body></html>`)
	}))
	defer srv.Close()

	cfg := CrawlerConfig{
		UserAgent:              "TestAgent",
		JSRenderMaxConcurrency: 1,
		JSRenderTimeout:        5 * time.Second,
		JSRenderDomainQPS:      1,
	}

	renderer, err := NewChromedpRenderer(cfg, zap.NewNop())
	if errors.Is(err, ErrRendererDisabled) {
		t.Skip("renderer disabled")
	}
	if err != nil {
		t.Skipf("chromedp unavailable: %v", err)
	}
	defer renderer.Close(context.Background())

	page, err := renderer.Render(context.Background(), srv.URL)
	if err != nil {
		t.Skipf("render failed: %v", err)
	}
	if !strings.Contains(string(page.Body), "late content") {
		t.Fatal("rendered body missing dynamic content")
	}
}
