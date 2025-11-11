package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMiddleware(t *testing.T) {
	Init()
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/notfound", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Make requests to the test server.
	resp, err := http.Get(ts.URL + "/test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errInner := resp.Body.Close(); errInner != nil {
			t.Log(errInner)
		}
	}()

	resp, err = http.Get(ts.URL + "/notfound")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errInner := resp.Body.Close(); errInner != nil {
			t.Log(errInner)
		}
	}()

	// Check the metrics.
	if val := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "200")); val != 1 {
		t.Errorf("Expected httpRequestsTotal for GET /test to be 1, got %f", val)
	}
	if val := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "404")); val != 1 {
		t.Errorf("Expected httpRequestsTotal for GET /notfound to be 1, got %f", val)
	}
	if val := testutil.CollectAndCount(httpRequestDurationSeconds); val <= 0 {
		t.Errorf("Expected httpRequestDurationSeconds to be observed, got %d", val)
	}
}
