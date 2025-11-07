package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- 1. Mock Implementations ---

// MockPublisher is a mock for the publisher interface
type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) Publish(ctx context.Context, msg *pubsub.Message) *pubsub.PublishResult {
	args := m.Called(ctx, msg)
	val := args.Get(0)
	if val == nil {
		return nil
	}
	result, ok := val.(*pubsub.PublishResult)
	if !ok {
		panic("MockPublisher.Publish expects *pubsub.PublishResult")
	}
	return result
}

func (m *MockPublisher) Stop() {
	m.Called()
}

// MockClientCloser is a mock for the clientCloser interface
type MockClientCloser struct {
	mock.Mock
}

func (m *MockClientCloser) Close() error {
	args := m.Called()
	if err := args.Error(0); err != nil {
		return fmt.Errorf("mock client close failed: %w", err)
	}
	return nil
}

// MockTopicAdmin is a mock for the topicAdmin interface
type MockTopicAdmin struct {
	mock.Mock
}

func (m *MockTopicAdmin) GetTopic(
	ctx context.Context,
	req *pubsubpb.GetTopicRequest,
	_ ...gax.CallOption,
) (*pubsubpb.Topic, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		if err := args.Error(1); err != nil {
			return nil, fmt.Errorf("mock topic lookup failed: %w", err)
		}
		return nil, nil
	}
	topic, ok := args.Get(0).(*pubsubpb.Topic)
	if !ok {
		panic("MockTopicAdmin.GetTopic expects *pubsubpb.Topic")
	}
	if err := args.Error(1); err != nil {
		return topic, fmt.Errorf("mock topic lookup failed: %w", err)
	}
	return topic, nil
}

// MockClientFactory is a mock for the ClientFactory interface
type MockClientFactory struct {
	mock.Mock
}

func (m *MockClientFactory) NewClients(ctx context.Context, projectID string) (*pubsub.Client, error) {
	// This mock is tricky since it returns concrete types.
	// We'll return nil for them as they are immediately wrapped or used
	// by the real NewPubSubProvider and we'll mock their behavior.
	// In a real test, we'd need a way to inject mockable *pubsub.Client.
	// For this refactor's test, we'll create a *separate* mock factory
	// that returns our mock interfaces.
	args := m.Called(ctx, projectID)
	if err := args.Error(2); err != nil {
		return nil, fmt.Errorf("mock client creation failed: %w", err)
	}
	return nil, nil
}

// --- 2. Test Suite ---

var testCtx = context.Background()

// TestPubSubProvider_Publish tests the Publish method
func TestPubSubProvider_Publish(t *testing.T) {
	// 1. Setup Mocks
	mockPub := &MockPublisher{}
	mockClient := &MockClientCloser{}
	provider := &PubSubProvider{
		publisher: mockPub,
		client:    mockClient,
	}
	crawlID := "12345"
	expectedMsg := &pubsub.Message{Data: []byte(crawlID)}
	mockResult := &pubsub.PublishResult{}

	// 2. Define Expectations
	mockPub.On("Publish", testCtx, expectedMsg).Return(mockResult).Once()

	// 3. Run Test
	err := provider.Publish(testCtx, crawlID)

	// 4. Assert
	assert.NoError(t, err)
	mockPub.AssertExpectations(t)
}

// TestPubSubProvider_Close tests the Close method
func TestPubSubProvider_Close(t *testing.T) {
	// 1. Setup Mocks
	mockPub := &MockPublisher{}
	mockClient := &MockClientCloser{}
	provider := &PubSubProvider{
		publisher: mockPub,
		client:    mockClient,
	}

	// 2. Define Expectations
	mockPub.On("Stop").Return().Once()
	mockClient.On("Close").Return(nil).Once()

	// 3. Run Test
	err := provider.Close()

	// 4. Assert
	assert.NoError(t, err)
	mockPub.AssertExpectations(t)
	mockClient.AssertExpectations(t)
	// We can also use mockery's Call.Once().NotBefore(otherCall) to check order.
}

// TestPubSubProvider_Close_Error tests the Close method when client fails
func TestPubSubProvider_Close_Error(t *testing.T) {
	// 1. Setup Mocks
	mockPub := &MockPublisher{}
	mockClient := &MockClientCloser{}
	provider := &PubSubProvider{
		publisher: mockPub,
		client:    mockClient,
	}
	expectedErr := errors.New("client close failure")

	// 2. Define Expectations
	mockPub.On("Stop").Return().Once()
	mockClient.On("Close").Return(expectedErr).Once()

	// 3. Run Test
	err := provider.Close()

	// 4. Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client close failure")
	mockPub.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}
