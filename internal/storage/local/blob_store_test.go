// Package local_test tests the local filesystem blob store.
package local_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/storage/local"
)

func TestNew(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := local.Config{BaseDir: tempDir}
		store, err := local.New(cfg)
		require.NoError(t, err)
		assert.NotNil(t, store)
	})
	t.Run("MissingBaseDir", func(t *testing.T) {
		cfg := local.Config{}
		_, err := local.New(cfg)
		assert.Error(t, err)
	})

	t.Run("BaseDirIsNotADirectory", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "testfile")
		require.NoError(t, err)
		t.Cleanup(func() {
			removeErr := os.Remove(tempFile.Name())
			if removeErr != nil && !os.IsNotExist(removeErr) {
				t.Fatalf("failed to remove temp file: %v", removeErr)
			}
		})

		cfg := local.Config{BaseDir: tempFile.Name()}
		_, err = local.New(cfg)
		assert.Error(t, err)
	})

	t.Run("BaseDirNotWritable", func(t *testing.T) {
		tempDir := t.TempDir()
		// Change permissions to read-only
		// #nosec G302 -- directory permissions adjusted intentionally for test coverage.
		err := os.Chmod(tempDir, 0o500)
		require.NoError(t, err)

		cfg := local.Config{BaseDir: tempDir}
		_, err = local.New(cfg)
		assert.Error(t, err)

		// Change back to writable so cleanup can happen
		// #nosec G302 -- reverting permissions to allow cleanup in the test environment.
		err = os.Chmod(tempDir, 0o700)
		require.NoError(t, err)
	})
}

func TestPutObject(t *testing.T) {
	tempDir := t.TempDir()
	cfg := local.Config{BaseDir: tempDir}
	store, err := local.New(cfg)
	require.NoError(t, err)

	t.Run("ValidPut", func(t *testing.T) {
		path := "test/object.txt"
		data := []byte("hello world")
		uri, err := store.PutObject(context.Background(), path, "text/plain", bytes.NewReader(data))
		require.NoError(t, err)

		expectedURI := "file://" + filepath.Join(tempDir, path)
		assert.Equal(t, expectedURI, uri)

		// Verify the file was written correctly.
		// #nosec G304 -- test reads from the controlled temp directory.
		readData, err := os.ReadFile(filepath.Join(tempDir, path))
		require.NoError(t, err)
		assert.Equal(t, data, readData)
	})

	t.Run("EmptyPath", func(t *testing.T) {
		_, err := store.PutObject(context.Background(), "", "text/plain", bytes.NewReader([]byte("data")))
		assert.Error(t, err)
	})

	t.Run("NestedPath", func(t *testing.T) {
		path := "a/b/c/object.txt"
		data := []byte("nested hello")
		uri, err := store.PutObject(context.Background(), path, "text/plain", bytes.NewReader(data))
		require.NoError(t, err)

		expectedURI := "file://" + filepath.Join(tempDir, path)
		assert.Equal(t, expectedURI, uri)

		// Verify the file was written correctly.
		// #nosec G304 -- test reads from the controlled temp directory.
		readData, err := os.ReadFile(filepath.Join(tempDir, path))
		require.NoError(t, err)
		assert.Equal(t, data, readData)
	})
}
