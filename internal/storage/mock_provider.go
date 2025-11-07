// Package storage defines the interfaces for a blob storage provider.
package storage

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockProvider is a mock implementation of the Provider interface for testing.
type MockProvider struct {
	mock.Mock
}

// Save is the mock implementation of the Save method.
func (m *MockProvider) Save(ctx context.Context, objectName string, data []byte) error {
	args := m.Called(ctx, objectName, data)
	return args.Error(0) //nolint:wrapcheck
}
