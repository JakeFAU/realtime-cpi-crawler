// Package queue defines the interfaces for a message queue provider.
// This abstraction allows the application to be independent of a specific
// message queue implementation (e.g., GCP Pub/Sub, RabbitMQ, Kafka).
package queue

import (
	"context"
)

// Provider defines the common interface for a message queue.
// It abstracts the operations of publishing messages and closing the connection.
type Provider interface {
	// Publish sends a message, identified by the unique crawl ID, to the configured topic.
	// This is often a non-blocking, asynchronous operation.
	Publish(ctx context.Context, crawlID string) error

	// Close cleans up any client connections and resources.
	Close() error
}

// NoOpProvider is a mock queue provider that performs no operations.
// It is useful for testing or running the application without a real message queue.
type NoOpProvider struct{}

// Publish for NoOpProvider does nothing and returns nil.
func (n *NoOpProvider) Publish(_ context.Context, _ string) error { return nil }

// Close for NoOpProvider does nothing and returns nil.
func (n *NoOpProvider) Close() error { return nil }
