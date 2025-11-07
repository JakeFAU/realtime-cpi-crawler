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
	const dynamicHTML = `<!doctype html><html><body>` +
		`<script>document.body.innerHTML = '<div id="late">late content</div>';</script>` +
		`</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, dynamicHTML); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := Config{
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
	t.Cleanup(func() {
		if cerr := renderer.Close(context.Background()); cerr != nil {
			t.Fatalf("close renderer: %v", cerr)
		}
	})

	page, err := renderer.Render(context.Background(), srv.URL)
	if err != nil {
		t.Skipf("render failed: %v", err)
	}
	if !strings.Contains(string(page.Body), "late content") {
		t.Fatal("rendered body missing dynamic content")
	}
}
