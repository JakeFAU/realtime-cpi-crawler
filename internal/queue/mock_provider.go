// Package queue defines the interfaces for a queue provider.
package queue

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockProvider is a mock implementation of the Provider interface for testing.
type MockProvider struct {
	mock.Mock
}

// Publish is the mock implementation of the Publish method.
func (m *MockProvider) Publish(ctx context.Context, crawlID string) error {
	args := m.Called(ctx, crawlID)
	return args.Error(0)
}

// Close is the mock implementation of the Close method.
func (m *MockProvider) Close() error {
	args := m.Called()
	return args.Error(0)
}
