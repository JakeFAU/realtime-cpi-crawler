package queue

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	gax "github.com/googleapis/gax-go/v2"
	"go.uber.org/zap"
)

// --- 1. Core Interfaces ---

// publisher defines the behavior for publishing a message.
// It is implemented by *pubsub.Publisher.
type publisher interface {
	Publish(ctx context.Context, msg *pubsub.Message) *pubsub.PublishResult
	Stop()
}

// clientCloser defines the behavior for closing a client connection.
// It is implemented by *pubsub.Client.
type clientCloser interface {
	Close() error
}

// topicAdmin defines the behavior for verifying a topic.
// It is implemented by *pubsub.TopicAdminClient.
type topicAdmin interface {
	GetTopic(ctx context.Context, req *pubsubpb.GetTopicRequest, opts ...gax.CallOption) (*pubsubpb.Topic, error)
}

// --- 2. Provider Implementation (Composed of Interfaces) ---

// PubSubProvider implements the queue.Provider interface using GCP Pub/Sub.
// It is composed of interfaces for testability.
type PubSubProvider struct {
	publisher publisher
	client    clientCloser
}

// Publish sends a message containing the crawlID to the Pub/Sub topic.
// This is a non-blocking, fire-and-forget operation.
func (p *PubSubProvider) Publish(ctx context.Context, crawlID string) error {
	msg := &pubsub.Message{
		Data: []byte(crawlID),
	}
	// Publish returns a "result" struct immediately. The actual send is asynchronous.
	result := p.publisher.Publish(ctx, msg)
	_ = result // Explicitly ignore the result to indicate fire-and-forget.
	return nil
}

// Close stops the topic's publisher and closes the underlying client connection.
func (p *PubSubProvider) Close() error {
	p.publisher.Stop() // Stop accepting new messages
	if err := p.client.Close(); err != nil {
		return fmt.Errorf("failed to close pubsub client: %w", err)
	}
	return nil
}

// --- 3. Factory for Concrete GCP Clients ---

// ClientFactory defines the interface for creating concrete GCP clients.
// This allows mocking the entire GCP client creation process.
type ClientFactory interface {
	NewClients(ctx context.Context, projectID string) (*pubsub.Client, error)
}

// DefaultClientFactory is the default implementation that creates real GCP clients.
type DefaultClientFactory struct{}

// NewClients creates a new Pub/Sub client and its associated TopicAdminClient.
func (f *DefaultClientFactory) NewClients(ctx context.Context, projectID string) (*pubsub.Client, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}
	// The pubsub.Client contains the TopicAdminClient internally.
	return client, nil
}

// --- 4. Constructor ---

func fullTopicName(projectID, topicID string) string {
	return fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
}

// NewPubSubProvider creates a new Pub/Sub provider.
// It uses the factory to create clients, verifies the topic, and returns a
// configured PubSubProvider.
func NewPubSubProvider(ctx context.Context, projectID, topicID string, factory ClientFactory) (*PubSubProvider, error) {
	// Use the factory to get the concrete clients
	client, err := factory.NewClients(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create pubsub client: %w", err)
	}

	admin := client.TopicAdminClient
	if admin == nil {
		if closeErr := client.Close(); closeErr != nil {
			logging.L.Warn("Failed to close pubsub client after missing admin client", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("pubsub client missing TopicAdminClient")
	}

	// Verify the topic exists using the admin client interface
	topic, err := verifyTopic(ctx, admin, projectID, topicID)
	if err != nil {
		// If verification fails, we must close the client we just created.
		if closeErr := client.Close(); closeErr != nil {
			logging.L.Warn("Failed to close pubsub client after topic verification failure", zap.Error(closeErr))
		}
		return nil, fmt.Errorf("verify pubsub topic: %w", err)
	}

	// Get a concrete publisher for the verified topic
	publisher := client.Publisher(topic.Name)

	// Return the provider, composed of the concrete types that match the interfaces
	return &PubSubProvider{
		publisher: publisher,
		client:    client,
	}, nil
}

// verifyTopic checks if a topic exists and is active.
func verifyTopic(ctx context.Context, admin topicAdmin, projectID, topicID string) (*pubsubpb.Topic, error) {
	request := &pubsubpb.GetTopicRequest{
		Topic: fullTopicName(projectID, topicID),
	}
	topic, err := admin.GetTopic(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get pubsub topic '%s': %w", topicID, err)
	}

	// Check if the topic exists and we have permission to publish to it.
	if topic.State != pubsubpb.Topic_ACTIVE {
		return nil, fmt.Errorf("pubsub topic '%s' in project '%s' is not active", topicID, projectID)
	}

	return topic, nil
}
