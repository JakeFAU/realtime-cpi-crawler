package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// FileSystemSink saves HTML and metadata to disk.
type FileSystemSink struct {
	root     string
	maxBytes int64
	logger   *zap.Logger
}

// NewFileSystemSink returns a sink rooted at dir.
func NewFileSystemSink(root string, maxBytes int64, logger *zap.Logger) (*FileSystemSink, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create sink dir: %w", err)
	}
	return &FileSystemSink{
		root:     root,
		maxBytes: maxBytes,
		logger:   logger,
	}, nil
}

// SaveHTML writes the HTML snapshot to disk.
func (s *FileSystemSink) SaveHTML(ctx context.Context, page Page) (string, error) {
	if page.ContentLength() == 0 {
		return "", fmt.Errorf("empty page body")
	}
	if int64(len(page.Body)) > s.maxBytes {
		return "", fmt.Errorf("page size %d exceeds max %d", len(page.Body), s.maxBytes)
	}
	target := htmlFilePath(s.root, page)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("creating HTML dir: %w", err)
	}
	if err := os.WriteFile(target, page.Body, 0o644); err != nil {
		return "", fmt.Errorf("writing HTML: %w", err)
	}
	return target, nil
}

// SaveMeta writes one metadata json per page.
func (s *FileSystemSink) SaveMeta(ctx context.Context, meta CrawlMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if meta.Path == "" {
		meta.Path = htmlFilePath(s.root, Page{URL: meta.URL, FinalURL: meta.FinalURL})
	}
	metaPath := s.metaPath(meta.Path)
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return fmt.Errorf("creating meta dir: %w", err)
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return os.WriteFile(metaPath, payload, 0o644)
}

func (s *FileSystemSink) metaPath(htmlPath string) string {
	return strings.TrimSuffix(htmlPath, filepath.Ext(htmlPath)) + ".json"
}
