package queue

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"go.uber.org/zap"
)

// PubSubProvider implements the queue.Provider interface for Google Cloud Pub/Sub.
type PubSubProvider struct {
	client *pubsub.Client
	topic  *pubsub.Topic
}

// NewPubSubProvider creates a new Pub/Sub client and gets a handle to the specified topic.
// It authenticates using Google Cloud's Application Default Credentials.
func NewPubSubProvider(ctx context.Context, projectID, topicID string) (*PubSubProvider, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	topic := client.Topic(topicID)

	// Check if the topic exists and we have permission to publish to it.
	exists, err := topic.Exists(ctx)
	if err != nil {
		err := client.Close()
		if err != nil {
			logging.L.Warn("Failed to close pubsub client after topic existence check failure", zap.Error(err))
		}
		return nil, fmt.Errorf("failed to check for topic existence: %w", err)
	}
	if !exists {
		err := client.Close()
		if err != nil {
			logging.L.Warn("Failed to close pubsub client after topic existence check failure", zap.Error(err))
		}
		return nil, fmt.Errorf("pubsub topic '%s' does not exist in project '%s'", topicID, projectID)
	}

	return &PubSubProvider{
		client: client,
		topic:  topic,
	}, nil
}

// Publish sends a message containing the crawlID to the Pub/Sub topic.
// This is a non-blocking, fire-and-forget operation. The Pub/Sub client
// handles batching, retries, and concurrency in the background.
func (p *PubSubProvider) Publish(ctx context.Context, crawlID string) error {
	msg := &pubsub.Message{
		Data: []byte(crawlID),
	}

	// Publish returns a "result" struct immediately. The actual send is asynchronous.
	result := p.topic.Publish(ctx, msg)

	// We can optionally wait for the result to be acknowledged by the server.
	// For a fire-and-forget approach, we don't block here.
	// _, err := result.Get(ctx)
	// return err

	_ = result // Explicitly ignore the result to indicate fire-and-forget.

	return nil
}

// Close stops the topic's publisher and closes the underlying client connection.
func (p *PubSubProvider) Close() error {
	p.topic.Stop()
	if err := p.client.Close(); err != nil {
		return fmt.Errorf("failed to close pubsub client: %w", err)
	}
	return nil
}
