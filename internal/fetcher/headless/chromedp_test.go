package headless

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

func TestNewChromedpLimiterValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewChromedp(Config{MaxParallel: -1}); err == nil {
		t.Fatal("expected error for negative max parallel")
	}
	fetcher, err := NewChromedp(Config{MaxParallel: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap(fetcher.limiter) != 2 {
		t.Fatalf("expected limiter capacity 2, got %d", cap(fetcher.limiter))
	}
}

func TestFetcherNavTimeoutDefault(t *testing.T) {
	t.Parallel()

	fetcher := &Fetcher{}
	if got := fetcher.navTimeout(); got != 45*time.Second {
		t.Fatalf("expected default nav timeout, got %v", got)
	}
	fetcher.cfg.NavigationTimeout = time.Second
	if got := fetcher.navTimeout(); got != time.Second {
		t.Fatalf("expected override to be used, got %v", got)
	}
}

func TestCloneHeaderAndNetworkHeaders(t *testing.T) {
	t.Parallel()

	src := http.Header{"X-Test": {"a", "b"}}
	cloned := cloneHeader(src)
	cloned.Add("X-Test", "c")
	if len(src["X-Test"]) != 2 {
		t.Fatalf("source header mutated: %+v", src)
	}

	netHeaders := toNetworkHeaders(src)
	switch v := netHeaders["X-Test"].(type) {
	case []string:
		if len(v) != 2 {
			t.Fatalf("expected two entries, got %v", v)
		}
	default:
		t.Fatalf("expected []string, got %T", v)
	}
}

func TestResponseMetaCaptureAndFallbacks(t *testing.T) {
	t.Parallel()

	meta := newResponseMeta()
	meta.capture(&network.EventResponseReceived{
		Type: network.ResourceTypeDocument,
		Response: &network.Response{
			Status:  204,
			URL:     "https://example.com/rendered",
			Headers: network.Headers{"X-Request-ID": "abc"},
		},
	})
	status, headers, url := meta.snapshotWithFallbacks("https://req", "")
	if status != 204 || headers.Get("X-Request-ID") != "abc" || url != "https://example.com/rendered" {
		t.Fatalf("unexpected snapshot values: status=%d headers=%v url=%s", status, headers, url)
	}

	meta = newResponseMeta()
	status, _, url = meta.snapshotWithFallbacks("https://req", "https://final")
	if status != http.StatusOK || url != "https://final" {
		t.Fatalf("expected fallback values, got status=%d url=%s", status, url)
	}
}

func TestNoopFetcherError(t *testing.T) {
	t.Parallel()

	fetcher := NewNoop()
	if _, err := fetcher.Fetch(context.Background(), crawler.FetchRequest{}); err == nil {
		t.Fatal("expected error from noop fetcher")
	}
}
