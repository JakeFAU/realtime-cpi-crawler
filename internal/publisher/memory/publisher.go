// Package memory contains in-memory publisher implementations for tests.
package memory

import (
	"context"
	"fmt"
	"sync"
)

// Publisher stores published payloads for inspection.
type Publisher struct {
	mu       sync.RWMutex
	messages []PublishedMessage
}

// PublishedMessage captures one publish call.
type PublishedMessage struct {
	Topic   string
	Payload any
}

// New returns a memory Publisher.
func New() *Publisher {
	return &Publisher{}
}

// Publish records the message and returns a pseudo ID.
func (p *Publisher) Publish(_ context.Context, topic string, payload any) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, PublishedMessage{Topic: topic, Payload: payload})
	return fmt.Sprintf("memory-%d", len(p.messages)), nil
}

// Messages returns the recorded publishes.
func (p *Publisher) Messages() []PublishedMessage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]PublishedMessage, len(p.messages))
	copy(out, p.messages)
	return out
}
