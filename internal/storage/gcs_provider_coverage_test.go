// Package storage_test contains unit tests for the storage package.
package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	appstorage "github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

// MockGCSClientFactory is a mock implementation of the GCSClientFactory interface.
// It allows for mocking the NewClient method.
type MockGCSClientFactory struct {
	Client *storage.Client
	Err    error
}

// NewClient returns the mock client and error.
func (m *MockGCSClientFactory) NewClient(_ context.Context) (*storage.Client, error) {
	return m.Client, m.Err
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewGCSProvider_Success(t *testing.T) {
	bucketName := "test-bucket"

	client, err := storage.NewClient(
		context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(&http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				assert.Contains(t, r.URL.Path, fmt.Sprintf("/storage/v1/b/%s", bucketName))
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			}),
		}),
	)

	require.NoError(t, err)

	factory := &MockGCSClientFactory{Client: client}

	provider, err := appstorage.NewGCSProvider(context.Background(), bucketName, factory)

	require.NoError(t, err)

	assert.NotNil(t, provider)
}

func TestNewGCSProvider_ClientError(t *testing.T) {
	factory := &MockGCSClientFactory{Err: fmt.Errorf("failed to create client")}

	_, err := appstorage.NewGCSProvider(context.Background(), "test-bucket", factory)

	require.Error(t, err)

	assert.Contains(t, err.Error(), "failed to create GCS client")
}

func TestNewGCSProvider_BucketAttrsError(t *testing.T) {
	bucketName := "test-bucket"

	// Create a context that times out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client, err := storage.NewClient(
		ctx, // Use the timeout context here
		option.WithoutAuthentication(),
		option.WithHTTPClient(&http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				assert.Contains(t, r.URL.Path, fmt.Sprintf("/storage/v1/b/%s", bucketName))
				return &http.Response{
					StatusCode: http.StatusInternalServerError, // This is fine now
					Body:       io.NopCloser(strings.NewReader(``)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			}),
		}),
	)

	require.NoError(t, err)

	factory := &MockGCSClientFactory{Client: client}

	// Pass the timeout context to the function under test
	_, err = appstorage.NewGCSProvider(ctx, bucketName, factory)

	require.Error(t, err) // It will now fail with a "context deadline exceeded" error
	assert.Contains(t, err.Error(), "failed to get GCS bucket")
}
