// Package storage_test contains unit tests for the storage package.
package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gcs "cloud.google.com/go/storage"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

// newTestGCSProvider creates a new GCSProvider pointed at a test server.
func newTestGCSProvider(t *testing.T, handler http.Handler) (*storage.GCSProvider, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	// Create a client that connects to the test server.
	// We also disable authentication for the test client.
	client, err := gcs.NewClient(context.Background(), option.WithEndpoint(server.URL), option.WithoutAuthentication())
	require.NoError(t, err)

	provider := &storage.GCSProvider{
		Client:     client,
		BucketName: "test-bucket",
	}

	// Return the provider and a cleanup function to close the server.
	return provider, server.Close
}

func TestGCSProvider_Save(t *testing.T) {
	objectName := "test-object"
	objectData := []byte("test-data")
	bucketName := "test-bucket"

	// This handler simulates the GCS JSON API for multipart uploads.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that the request is for the correct bucket and object.
		assert.Contains(t, r.URL.Path, fmt.Sprintf("/upload/storage/v1/b/%s/o", bucketName))
		assert.Equal(t, objectName, r.URL.Query().Get("name"))
		assert.Equal(t, "multipart", r.URL.Query().Get("uploadType"))

		// Read the body to ensure the data was sent correctly.
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), string(objectData))

		// Respond with a success message.
		fmt.Fprintln(w, `{ "name": "`+objectName+`" }`)
	})

	provider, cleanup := newTestGCSProvider(t, handler)
	defer cleanup()

	// Call the Save method.
	err := provider.Save(context.Background(), objectName, objectData)
	assert.NoError(t, err)
}

func TestGCSProvider_Save_Error(t *testing.T) {
	objectName := "test-object"
	objectData := []byte("test-data")

	// This handler simulates a server error.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	provider, cleanup := newTestGCSProvider(t, handler)
	defer cleanup()

	// Call the Save method and expect an error.
	err := provider.Save(context.Background(), objectName, objectData)
	assert.Error(t, err)
}
