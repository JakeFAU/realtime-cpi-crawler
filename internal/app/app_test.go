// Package app_test contains unit tests for the app package.
package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/app"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockDatabaseProvider mocks the database.Provider interface.
type MockDatabaseProvider struct {
	mock.Mock
}

// SaveCrawl satisfies the database.Provider interface for the mock.
func (m *MockDatabaseProvider) SaveCrawl(ctx context.Context, meta database.CrawlMetadata) (string, error) {
	args := m.Called(ctx, meta)
	return args.String(0), args.Error(1)
}

// Close satisfies the database.Provider interface for the mock.
func (m *MockDatabaseProvider) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockQueueProvider mocks the queue.Provider interface.
type MockQueueProvider struct {
	mock.Mock
}

// Publish satisfies the queue.Provider interface for the mock.
func (m *MockQueueProvider) Publish(ctx context.Context, crawlID string) error {
	args := m.Called(ctx, crawlID)
	return args.Error(0)
}

// Close satisfies the queue.Provider interface for the mock.
func (m *MockQueueProvider) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestMain(m *testing.M) {
	// Initialize the logger for all tests in this package.
	logging.InitLogger()
	// Run the tests.
	m.Run()
}

// setupTest configures Viper with default "noop" providers for a clean test environment.
func setupTest() {
	viper.Reset()
	viper.Set("storage.provider", "noop")
	viper.Set("database.provider", "noop")
	viper.Set("queue.provider", "noop")
}

func TestNewApp_Success(t *testing.T) {
	setupTest()
	ctx := context.Background()

	a, err := app.NewApp(ctx)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.NotNil(t, a.Logger)
	assert.IsType(t, &storage.NoOpProvider{}, a.Storage)
	assert.IsType(t, &database.NoOpProvider{}, a.Database)
	assert.IsType(t, &queue.NoOpProvider{}, a.Queue)
}

func TestNewApp_ConfigErrors(t *testing.T) {
	testCases := []struct {
		name          string
		configSetup   func()
		expectedError string
	}{
		{
			name: "GCS storage missing bucket",
			configSetup: func() {
				viper.Set("storage.provider", "gcs")
				viper.Set("storage.gcs.bucket_name", "")
			},
			expectedError: "storage provider is 'gcs' but storage.gcs.bucket_name is not set",
		},
		{
			name: "Postgres database missing DSN",
			configSetup: func() {
				viper.Set("database.provider", "postgres")
				viper.Set("database.postgres.dsn", "")
			},
			expectedError: "database provider is 'postgres' but database.postgres.dsn is not set",
		},
		{
			name: "Pub/Sub queue missing project ID",
			configSetup: func() {
				viper.Set("queue.provider", "pubsub")
				viper.Set("queue.gcp.project_id", "")
				viper.Set("queue.gcp.topic_id", "test-topic")
			},
			expectedError: "queue provider is 'pubsub' but project_id or topic_id is not set",
		},
		{
			name: "Unknown storage provider",
			configSetup: func() {
				viper.Set("storage.provider", "unknown")
			},
			expectedError: "unknown storage provider: unknown",
		},
		{
			name: "Unknown database provider",
			configSetup: func() {
				viper.Set("database.provider", "unknown")
			},
			expectedError: "unknown database provider: unknown",
		},
		{
			name: "Unknown queue provider",
			configSetup: func() {
				viper.Set("queue.provider", "unknown")
			},
			expectedError: "unknown queue provider: unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupTest()
			tc.configSetup()
			ctx := context.Background()

			_, err := app.NewApp(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

func TestApp_Close(t *testing.T) {
	dbMock := new(MockDatabaseProvider)
	qMock := new(MockQueueProvider)

	// Expect Close to be called and return no error.
	dbMock.On("Close").Return(nil).Once()
	qMock.On("Close").Return(nil).Once()

	a := &app.App{
		Logger:   logging.L,
		Database: dbMock,
		Queue:    qMock,
	}

	a.Close()

	dbMock.AssertExpectations(t)
	qMock.AssertExpectations(t)
}

func TestApp_Close_WithErrors(t *testing.T) {
	dbMock := new(MockDatabaseProvider)
	qMock := new(MockQueueProvider)

	// Expect Close to be called and return an error.
	dbMock.On("Close").Return(errors.New("db error")).Once()
	qMock.On("Close").Return(errors.New("queue error")).Once()

	a := &app.App{
		Logger:   logging.L,
		Database: dbMock,
		Queue:    qMock,
	}

	a.Close()

	dbMock.AssertExpectations(t)
	qMock.AssertExpectations(t)
}
