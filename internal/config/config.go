// Package config loads and validates crawler configuration via Viper.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
)

// Config captures all service configuration knobs loaded via Viper.
type Config struct {
	Server       ServerConfig                     `mapstructure:"server"`
	Auth         AuthConfig                       `mapstructure:"auth"`
	Crawler      CrawlerConfig                    `mapstructure:"crawler"`
	HTTP         HTTPConfig                       `mapstructure:"http"`
	Headless     HeadlessConfig                   `mapstructure:"headless"`
	Storage      StorageConfig                    `mapstructure:"storage"`
	PubSub       PubSubConfig                     `mapstructure:"pubsub"`
	Logging      LoggingConfig                    `mapstructure:"logging"`
	StandardJobs map[string]crawler.JobParameters `mapstructure:"standard_jobs"`
}

// ServerConfig controls HTTP server behavior.
type ServerConfig struct {
	Port int `mapstructure:"port"`
}

// AuthConfig defines API authentication toggles.
type AuthConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key"`
}

// CrawlerConfig governs dispatcher and crawl pipeline behavior.
type CrawlerConfig struct {
	Concurrency      int    `mapstructure:"concurrency"`
	UserAgent        string `mapstructure:"user_agent"`
	IgnoreRobots     bool   `mapstructure:"ignore_robots"`
	MaxDepthDefault  int    `mapstructure:"max_depth_default"`
	MaxPagesDefault  int    `mapstructure:"max_pages_default"`
	GlobalQueueDepth int    `mapstructure:"queue_depth"`
}

// HTTPConfig configures HTTP client retry behavior.
type HTTPConfig struct {
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
}

// HeadlessConfig configures the headless rendering subsystem.
type HeadlessConfig struct {
	Enabled         bool `mapstructure:"enabled"`
	MaxParallel     int  `mapstructure:"max_parallel"`
	NavTimeoutSec   int  `mapstructure:"nav_timeout_seconds"`
	PromotionThresh int  `mapstructure:"promotion_threshold"`
}

// StorageConfig sets paths and content types for blob persistence.
type StorageConfig struct {
	Backend     string `mapstructure:"backend"`
	Bucket      string `mapstructure:"bucket"`
	Prefix      string `mapstructure:"prefix"`
	ContentType string `mapstructure:"content_type"`
}

// PubSubConfig holds metadata for publish-subscribe notifications.
type PubSubConfig struct {
	TopicName string `mapstructure:"topic_name"`
}

// LoggingConfig toggles zap development features.
type LoggingConfig struct {
	Development bool `mapstructure:"development"`
}

// Load builds a Config from disk/environment.
func Load(path string) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("CRAWLER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("crawler.concurrency", 4)
	v.SetDefault("crawler.user_agent", "real-cpi-bot/0.1")
	v.SetDefault("crawler.ignore_robots", false)
	v.SetDefault("crawler.max_depth_default", 1)
	v.SetDefault("crawler.max_pages_default", 10)
	v.SetDefault("crawler.queue_depth", 64)
	v.SetDefault("http.timeout_seconds", 15)
	v.SetDefault("headless.enabled", false)
	v.SetDefault("headless.max_parallel", 1)
	v.SetDefault("headless.nav_timeout_seconds", 25)
	v.SetDefault("headless.promotion_threshold", 2048)
	v.SetDefault("storage.backend", "memory")
	v.SetDefault("storage.prefix", "crawl")
	v.SetDefault("storage.content_type", "text/html; charset=utf-8")
	v.SetDefault("logging.development", true)
}

// Validate enforces required values and reasonable limits.
func (c Config) Validate() error {
	if c.Server.Port <= 0 {
		return fmt.Errorf("server.port must be > 0")
	}
	if c.Crawler.Concurrency <= 0 {
		return fmt.Errorf("crawler.concurrency must be > 0")
	}
	if c.Crawler.GlobalQueueDepth <= 0 {
		return fmt.Errorf("crawler.queue_depth must be > 0")
	}
	if c.Crawler.MaxDepthDefault < 0 {
		return fmt.Errorf("crawler.max_depth_default must be >= 0")
	}
	if c.Crawler.MaxPagesDefault <= 0 {
		return fmt.Errorf("crawler.max_pages_default must be > 0")
	}
	if c.HTTP.TimeoutSeconds <= 0 {
		return fmt.Errorf("http.timeout_seconds must be > 0")
	}
	if c.Headless.Enabled && c.Headless.MaxParallel <= 0 {
		return fmt.Errorf("headless.max_parallel must be > 0 when headless is enabled")
	}
	if c.Auth.Enabled && c.Auth.APIKey == "" {
		return fmt.Errorf("auth.api_key must be set when auth is enabled")
	}
	switch c.Storage.Backend {
	case "memory":
	case "gcs":
		if strings.TrimSpace(c.Storage.Bucket) == "" {
			return fmt.Errorf("storage.bucket must be set when storage.backend is gcs")
		}
	default:
		return fmt.Errorf("storage.backend must be either memory or gcs")
	}
	return nil
}

// JobBudget converts the HTTP timeout/backoff config into duration helpers.
func (c Config) JobBudget() time.Duration {
	return time.Duration(c.HTTP.TimeoutSeconds) * time.Second
}
