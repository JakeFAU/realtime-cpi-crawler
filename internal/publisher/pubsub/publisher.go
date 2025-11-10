// Package pubsub implements a Google Cloud Pub/Sub publisher.
package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"cloud.google.com/go/pubsub"
)

// Publisher wraps a Pub/Sub topic.
type Publisher struct {
	topic *pubsub.Topic
}

// New creates a Publisher for the provided topic.
func New(topic *pubsub.Topic) *Publisher {
	return &Publisher{topic: topic}
}

// Publish marshals the payload to JSON and publishes it to the topic.
func (p *Publisher) Publish(ctx context.Context, _ string, payload any) (string, error) {
	if p.topic == nil {
		return "", fmt.Errorf("pubsub topic is not configured")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	result := p.topic.Publish(ctx, &pubsub.Message{Data: data})
	id, err := result.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("publish message: %w", err)
	}
	return id, nil
}
