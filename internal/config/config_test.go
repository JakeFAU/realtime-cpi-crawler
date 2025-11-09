package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
  per_domain_max: 3
  user_agent: real-agent
  delay_seconds: 2
  ignore_robots: true
  max_depth_default: 5
  max_pages_default: 50
  queue_depth: 128
http:
  timeout_seconds: 45
  max_retries: 4
  backoff_initial_ms: 100
  backoff_max_ms: 500
headless:
  enabled: true
  max_parallel: 2
  nav_timeout_seconds: 30
  promotion_threshold: 70
storage:
  gcs_bucket: bucket
  prefix: logs
  content_type: text/plain
logging:
  development: false
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

func TestConfigValidateErrors(t *testing.T) {
	t.Parallel()

	base := Config{
		Server:  ServerConfig{Port: 8080},
		Crawler: CrawlerConfig{Concurrency: 1},
		HTTP:    HTTPConfig{TimeoutSeconds: 10},
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
