// Package config loads and validates crawler configuration via Viper.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/storage/local"
)

// Config captures all service configuration knobs loaded via Viper.
type Config struct {
	Server       ServerConfig                     `mapstructure:"server"`
	Auth         AuthConfig                       `mapstructure:"auth"`
	Crawler      CrawlerConfig                    `mapstructure:"crawler"`
	HTTP         HTTPConfig                       `mapstructure:"http"`
	Headless     HeadlessConfig                   `mapstructure:"headless"`
	Storage      StorageConfig                    `mapstructure:"storage"`
	Database     DatabaseConfig                   `mapstructure:"database"`
	PubSub       PubSubConfig                     `mapstructure:"pubsub"`
	Logging      LoggingConfig                    `mapstructure:"logging"`
	Metrics      MetricsConfig                    `mapstructure:"metrics"`
	Progress     ProgressConfig                   `mapstructure:"progress"`
	StandardJobs map[string]crawler.JobParameters `mapstructure:"standard_jobs"`
}

// MetricsConfig controls Prometheus metrics exposition.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
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
	Backend     string       `mapstructure:"backend"`
	Bucket      string       `mapstructure:"bucket"`
	Prefix      string       `mapstructure:"prefix"`
	ContentType string       `mapstructure:"content_type"`
	Local       local.Config `mapstructure:"local"`
}

// DatabaseConfig controls Postgres connectivity for retrieval persistence.
type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Name            string        `mapstructure:"name"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxConns        int32         `mapstructure:"max_conns"`
	MinConns        int32         `mapstructure:"min_conns"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	RetrievalTable  string        `mapstructure:"retrieval_table"`
	ProgressTable   string        `mapstructure:"progress_table"`
	StatsTable      string        `mapstructure:"stats_table"`
}

func (c *DatabaseConfig) resolveDSN() error { //nolint:gocyclo // helper enumerates validation branches for clarity
	if c == nil {
		return nil
	}
	if strings.TrimSpace(c.DSN) != "" {
		return nil
	}
	if strings.TrimSpace(c.Host) == "" &&
		c.Port == 0 &&
		strings.TrimSpace(c.User) == "" &&
		strings.TrimSpace(c.Password) == "" &&
		strings.TrimSpace(c.Name) == "" &&
		strings.TrimSpace(c.SSLMode) == "" {
		return nil
	}
	missing := make([]string, 0, 4)
	if strings.TrimSpace(c.Host) == "" {
		missing = append(missing, "database.host")
	}
	if strings.TrimSpace(c.User) == "" {
		missing = append(missing, "database.user")
	}
	if strings.TrimSpace(c.Password) == "" {
		missing = append(missing, "database.password")
	}
	if strings.TrimSpace(c.Name) == "" {
		missing = append(missing, "database.name")
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"database config incomplete: set %s when database.dsn is omitted",
			strings.Join(missing, ", "),
		)
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	// Normalize c.Host: remove brackets if present (for IPv6)
	host := c.Host
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   hostPort,
		Path:   "/" + c.Name,
	}
	query := url.Values{}
	if strings.TrimSpace(c.SSLMode) != "" {
		query.Set("sslmode", c.SSLMode)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	c.DSN = u.String()
	return nil
}

// PubSubConfig holds metadata for publish-subscribe notifications.
type PubSubConfig struct {
	TopicName string `mapstructure:"topic_name"`
	ProjectID string `mapstructure:"project_id"`
}

// LoggingConfig toggles zap development features.
type LoggingConfig struct {
	Development bool `mapstructure:"development"`
}

// ProgressConfig controls the progress hub batching behavior.
// Environment variables (via CRAWLER_ prefix):
//
//	CRAWLER_PROGRESS_ENABLED
//	CRAWLER_PROGRESS_BUFFER_SIZE
//	CRAWLER_PROGRESS_BATCH_MAX_EVENTS
//	CRAWLER_PROGRESS_BATCH_MAX_WAIT_MS
//	CRAWLER_PROGRESS_SINK_TIMEOUT_MS
//	CRAWLER_PROGRESS_LOG_ENABLED
type ProgressConfig struct {
	// Enabled toggles the progress subsystem (default true).
	Enabled bool `mapstructure:"enabled"`
	// BufferSize configures hub channel capacity (default 4096).
	BufferSize int `mapstructure:"buffer_size"`
	// Batch controls flush thresholds.
	Batch ProgressBatchConfig `mapstructure:"batch"`
	// SinkTimeoutMs bounds per-sink Consume calls (default 2000).
	SinkTimeoutMs int `mapstructure:"sink_timeout_ms"`
	// LogEnabled toggles the LogSink for debugging (default false).
	LogEnabled bool `mapstructure:"log_enabled"`
}

// ProgressBatchConfig contains batching thresholds.
type ProgressBatchConfig struct {
	// MaxEvents flushes after this many events accumulate (default 1000).
	MaxEvents int `mapstructure:"max_events"`
	// MaxWaitMs flushes after this many milliseconds even if under MaxEvents (default 500).
	MaxWaitMs int `mapstructure:"max_wait_ms"`
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

	if err := cfg.Database.resolveDSN(); err != nil {
		return Config{}, err
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
	v.SetDefault("storage.local.base_dir", "tmp/storage")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.table", "retrievals")
	v.SetDefault("logging.development", true)
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("progress.enabled", true)
	v.SetDefault("progress.buffer_size", 4096)
	v.SetDefault("progress.batch.max_events", 1000)
	v.SetDefault("progress.batch.max_wait_ms", 500)
	v.SetDefault("progress.sink_timeout_ms", 2000)
	v.SetDefault("progress.log_enabled", false)
}

// Validate enforces required values and reasonable limits.
//
//nolint:gocyclo,gocognit // reason (Configuration validation)
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
	case "local":
		if strings.TrimSpace(c.Storage.Local.BaseDir) == "" {
			return fmt.Errorf("storage.local.base_dir must be set when storage.backend is local")
		}
	default:
		return fmt.Errorf("storage.backend must be either memory, gcs, or local")
	}
	if c.Database.DSN != "" && strings.TrimSpace(c.Database.RetrievalTable) == "" {
		return fmt.Errorf("database.retrieval_table must be set when database.dsn is provided")
	}
	if c.Database.DSN != "" && strings.TrimSpace(c.Database.ProgressTable) == "" {
		return fmt.Errorf("database.progress_table must be set when database.dsn is provided")
	}
	if c.Database.DSN != "" && strings.TrimSpace(c.Database.StatsTable) == "" {
		return fmt.Errorf("database.stats_table must be set when database.dsn is provided")
	}
	if c.PubSub.TopicName != "" && strings.TrimSpace(c.PubSub.ProjectID) == "" {
		return fmt.Errorf("pubsub.project_id must be set when pubsub.topic_name is configured")
	}
	if c.Progress.Enabled {
		if c.Progress.BufferSize <= 0 {
			return fmt.Errorf("progress.buffer_size must be > 0")
		}
		if c.Progress.Batch.MaxEvents <= 0 {
			return fmt.Errorf("progress.batch.max_events must be > 0")
		}
		if c.Progress.Batch.MaxWaitMs <= 0 {
			return fmt.Errorf("progress.batch.max_wait_ms must be > 0")
		}
		if c.Progress.SinkTimeoutMs <= 0 {
			return fmt.Errorf("progress.sink_timeout_ms must be > 0")
		}
	}
	return nil
}

// JobBudget converts the HTTP timeout/backoff config into duration helpers.
func (c Config) JobBudget() time.Duration {
	return time.Duration(c.HTTP.TimeoutSeconds) * time.Second
}
