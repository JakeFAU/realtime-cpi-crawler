// Package local implements a local filesystem blob store.
package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Config captures the parameters for the local filesystem blob store.
type Config struct {
	// BaseDir is the root directory where blobs will be stored.
	BaseDir string `mapstructure:"base_dir" yaml:"base_dir"`
}

// BlobStore writes artifacts to the local filesystem.
type BlobStore struct {
	baseDir string
}

// New creates a new local filesystem-backed blob store.
func New(cfg Config) (*BlobStore, error) {
	if strings.TrimSpace(cfg.BaseDir) == "" {
		return nil, fmt.Errorf("base directory is required")
	}

	// Check if the directory exists and is writable.
	info, err := os.Stat(cfg.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist, try to create it.
			if mkErr := os.MkdirAll(cfg.BaseDir, 0o750); mkErr != nil {
				return nil, fmt.Errorf("failed to create base directory: %w", mkErr)
			}
		} else {
			// Some other error.
			return nil, fmt.Errorf("failed to stat base directory: %w", err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("base directory path is not a directory")
	}

	// Check for write permissions.
	testFile := filepath.Join(cfg.BaseDir, ".writable_test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		return nil, fmt.Errorf("base directory is not writable: %w", err)
	}
	if err := os.Remove(testFile); err != nil {
		return nil, fmt.Errorf("failed to clean up test file: %w", err)
	}

	return &BlobStore{
		baseDir: cfg.BaseDir,
	}, nil
}

// PutObject writes data to a file on the local filesystem and returns a file:// URI.
func (s *BlobStore) PutObject(_ context.Context, path string, _ string, data io.Reader) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	fullPath := filepath.Join(s.baseDir, path)

	// Clean the path and verify it's within baseDir to prevent path traversal.
	cleanBaseDir := filepath.Clean(s.baseDir)
	cleanFullPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanFullPath, cleanBaseDir+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal detected")
	}
	// Create parent directories if they don't exist.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create parent directories: %w", err)
	}

	// Read data from io.Reader
	byteData, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("failed to read data from reader: %w", err)
	}

	// Write the file.
	err := os.WriteFile(fullPath, byteData, 0o600) // Use byteData here
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("file://%s", fullPath), nil
}
