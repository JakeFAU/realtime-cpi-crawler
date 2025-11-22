// Package metrics provides middleware for HTTP metrics.
package metrics

import (
	"net/http"
	"time"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
	"github.com/go-chi/chi/v5"
)

// Middleware is a chi middleware that records HTTP request metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = "unknown"
		}

		telemetry.ObserveHTTPRequest(r.Method, routePattern, ww.status, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
