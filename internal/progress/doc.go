// Package progress provides the event primitives, non-blocking hub, and emitter
// interfaces that workers use to report crawl progress. It batches events on a
// background goroutine and fans them out to pluggable sinks such as Prometheus
// metrics or persistent storage.
package progress
