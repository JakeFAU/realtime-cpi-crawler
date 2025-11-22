package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
)

func TestLimiter_Wait(t *testing.T) {
	cfg := config.Config{}
	if _, _, err := telemetry.InitTelemetry(context.Background(), &cfg); err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}
	// Create a limiter with 1 RPS and burst 1
	l := New(Config{
		DefaultRPS:   10,
		DefaultBurst: 1,
	})

	ctx := context.Background()
	url := "https://example.com/foo"

	// First call should be immediate
	start := time.Now()
	if err := l.Wait(ctx, url); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Logf("warning: first wait took %v", time.Since(start))
	}

	// Second call should block if we were using 1 RPS, but we used 10 RPS.
	// 10 RPS = 1 token every 100ms.
	// Burst 1 means we start with 1 token.
	// After first consume, we have 0.
	// Next token in 100ms.

	// Let's use a stricter limit for testing delay.
	l2 := New(Config{
		DefaultRPS:   10, // 10 requests per second = 100ms interval
		DefaultBurst: 1,
	})

	// Consume initial token
	if err := l2.Wait(ctx, "https://test.com"); err != nil {
		t.Fatal(err)
	}

	// Next one should wait ~100ms
	start = time.Now()
	if err := l2.Wait(ctx, "https://test.com"); err != nil {
		t.Fatal(err)
	}
	dur := time.Since(start)
	if dur < 80*time.Millisecond {
		t.Errorf("expected wait ~100ms, got %v", dur)
	}
}

func TestLimiter_DifferentDomains(t *testing.T) {
	cfg := config.Config{}
	if _, _, err := telemetry.InitTelemetry(context.Background(), &cfg); err != nil {
		t.Fatalf("failed to init telemetry: %v", err)
	}
	// Create a limiter with 1 RPS and burst 1
	l := New(Config{
		DefaultRPS:   1, // 1 RPS = 1s interval
		DefaultBurst: 1,
	})

	ctx := context.Background()

	// Domain A
	if err := l.Wait(ctx, "https://a.com/1"); err != nil {
		t.Fatal(err)
	}

	// Domain B should not be blocked by A
	start := time.Now()
	if err := l.Wait(ctx, "https://b.com/1"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 10*time.Millisecond {
		t.Errorf("domain B blocked unexpectedly")
	}
}
