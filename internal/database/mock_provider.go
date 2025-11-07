// Package database defines the interfaces for a database provider.
package database

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockProvider is a mock implementation of the Provider interface for testing.
type MockProvider struct {
	mock.Mock
}

// SaveCrawl is the mock implementation of the SaveCrawl method.
func (m *MockProvider) SaveCrawl(ctx context.Context, meta CrawlMetadata) (string, error) {
	args := m.Called(ctx, meta)
	return args.String(0), args.Error(1)
}

// Close is the mock implementation of the Close method.
func (m *MockProvider) Close() error {
	args := m.Called()
	return args.Error(0) //nolint:wrapcheck
}
