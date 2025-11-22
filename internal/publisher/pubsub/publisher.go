// Package pubsub implements a Google Cloud Pub/Sub publisher.
package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	pubsub "cloud.google.com/go/pubsub/v2"
	"go.opentelemetry.io/otel"
)

// Publisher wraps a Pub/Sub publisher client.
type Publisher struct {
	publisher *pubsub.Publisher
}

// New creates a Publisher for the provided topic publisher.
func New(publisher *pubsub.Publisher) *Publisher {
	return &Publisher{publisher: publisher}
}

// Publish marshals the payload to JSON and publishes it to the topic.
func (p *Publisher) Publish(ctx context.Context, _ string, payload any) (string, error) {
	if p.publisher == nil {
		return "", fmt.Errorf("pubsub publisher is not configured")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	msg := &pubsub.Message{Data: data}
	msg.Attributes = make(map[string]string)
	otel.GetTextMapPropagator().Inject(ctx, &pubsubCarrier{attrs: msg.Attributes})

	result := p.publisher.Publish(ctx, msg)
	id, err := result.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("publish message: %w", err)
	}
	return id, nil
}

// pubsubCarrier implements propagation.TextMapCarrier for Pub/Sub attributes.
type pubsubCarrier struct {
	attrs map[string]string
}

func (c *pubsubCarrier) Get(key string) string {
	return c.attrs[key]
}

func (c *pubsubCarrier) Set(key, value string) {
	c.attrs[key] = value
}

func (c *pubsubCarrier) Keys() []string {
	keys := make([]string, 0, len(c.attrs))
	for k := range c.attrs {
		keys = append(keys, k)
	}
	return keys
}
