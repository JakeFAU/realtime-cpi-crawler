package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestRobotsEnforcer(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	allowAll := NewRobotsEnforcer(false, "test-agent", logger)
	if !allowAll.Allowed(ctx, "https://example.com/whatever") {
		t.Fatal("allow-all policy should permit URLs")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fmt.Fprintln(w, "User-agent: *\nDisallow: /blocked")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	enforcer := NewRobotsEnforcer(true, "test-agent", logger)
	if !enforcer.Allowed(ctx, srv.URL+"/allowed") {
		t.Fatal("expected allowed path to pass robots")
	}
	if enforcer.Allowed(ctx, srv.URL+"/blocked") {
		t.Fatal("expected blocked path to be denied")
	}
}
