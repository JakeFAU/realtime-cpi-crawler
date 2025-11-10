// Package sinks implements concrete progress consumers such as Prometheus,
// repository-backed storage, and structured logging. Each sink satisfies the
// progress.Sink interface and is safe for repeated Consume/Close cycles.
package sinks
