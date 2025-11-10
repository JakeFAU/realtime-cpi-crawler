// Package config provides tests covering configuration loading and validation.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoadWithFileOverrides confirms file overrides and defaults merge correctly.
//
//gocyclo:ignore
func TestLoadWithFileOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	configYAML := `
server:
  port: 9090
auth:
  enabled: true
  api_key: secret
crawler:
  concurrency: 6
  user_agent: real-agent
  ignore_robots: true
  max_depth_default: 5
  max_pages_default: 50
  queue_depth: 128
http:
  timeout_seconds: 45
headless:
  enabled: true
  max_parallel: 2
  nav_timeout_seconds: 30
  promotion_threshold: 70
storage:
  backend: gcs
  bucket: crawler
  prefix: logs
  content_type: text/plain
logging:
  development: false
pubsub:
  topic_name: topic
standard_jobs:
  price-refresh:
    urls: ["https://example.com"]
    headless_allowed: true
    respect_robots: true
`
	if err := os.WriteFile(path, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Server.Port)
	}
	if !cfg.Auth.Enabled || cfg.Auth.APIKey != "secret" {
		t.Fatalf("expected auth enabled with secret key")
	}
	if cfg.Crawler.Concurrency != 6 || cfg.Crawler.IgnoreRobots != true {
		t.Fatalf("expected crawler overrides to apply")
	}
	if cfg.Storage.Backend != "gcs" || cfg.Storage.Bucket != "crawler" {
		t.Fatalf("expected gcs storage config, got %+v", cfg.Storage)
	}
	job, ok := cfg.StandardJobs["price-refresh"]
	if !ok || len(job.URLs) != 1 || job.URLs[0] != "https://example.com" {
		t.Fatalf("expected standard job to be loaded: %+v", cfg.StandardJobs)
	}
	if job.HeadlessAllowed != true || job.RespectRobots != true {
		t.Fatalf("expected job booleans to be preserved: %+v", job)
	}
	if got := cfg.JobBudget(); got != 45*time.Second {
		t.Fatalf("expected job budget 45s, got %v", got)
	}
}

// TestConfigValidateErrors enumerates validation failures for invalid settings.
func TestConfigValidateErrors(t *testing.T) {
	t.Parallel()

	base := Config{
		Server: ServerConfig{Port: 8080},
		Crawler: CrawlerConfig{
			Concurrency:      1,
			MaxDepthDefault:  1,
			MaxPagesDefault:  10,
			GlobalQueueDepth: 10,
		},
		HTTP:    HTTPConfig{TimeoutSeconds: 10},
		Storage: StorageConfig{Backend: "memory"},
	}

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "invalid port",
			cfg: func() Config {
				c := base
				c.Server.Port = 0
				return c
			}(),
			want: "server.port",
		},
		{
			name: "invalid concurrency",
			cfg: func() Config {
				c := base
				c.Crawler.Concurrency = 0
				return c
			}(),
			want: "crawler.concurrency",
		},
		{
			name: "invalid queue depth",
			cfg: func() Config {
				c := base
				c.Crawler.GlobalQueueDepth = 0
				return c
			}(),
			want: "crawler.queue_depth",
		},
		{
			name: "invalid max pages default",
			cfg: func() Config {
				c := base
				c.Crawler.MaxPagesDefault = 0
				return c
			}(),
			want: "crawler.max_pages_default",
		},
		{
			name: "invalid max depth default",
			cfg: func() Config {
				c := base
				c.Crawler.MaxDepthDefault = -1
				return c
			}(),
			want: "crawler.max_depth_default",
		},
		{
			name: "invalid timeout",
			cfg: func() Config {
				c := base
				c.HTTP.TimeoutSeconds = 0
				return c
			}(),
			want: "http.timeout_seconds",
		},
		{
			name: "headless missing max parallel",
			cfg: func() Config {
				c := base
				c.Headless.Enabled = true
				c.Headless.MaxParallel = 0
				return c
			}(),
			want: "headless.max_parallel",
		},
		{
			name: "auth missing api key",
			cfg: func() Config {
				c := base
				c.Auth.Enabled = true
				return c
			}(),
			want: "auth.api_key",
		},
		{
			name: "unknown storage backend",
			cfg: func() Config {
				c := base
				c.Storage.Backend = "fs"
				return c
			}(),
			want: "storage.backend",
		},
		{
			name: "gcs missing bucket",
			cfg: func() Config {
				c := base
				c.Storage.Backend = "gcs"
				c.Storage.Bucket = ""
				return c
			}(),
			want: "storage.bucket",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}
