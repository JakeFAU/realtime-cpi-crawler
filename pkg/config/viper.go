// Package config is responsible for initializing the application's configuration.
// It uses the Viper library to read settings from a config file, environment
// variables, and command-line flags, providing a unified configuration system.
package config

import (
	"strings"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// InitConfig initializes the application's configuration using Viper.
// It sets up default values, defines configuration search paths, and enables
// reading from environment variables. This function is designed to be called
// once at application startup to ensure that configuration is loaded and
// available to all other packages.
func InitConfig() {
	// --- Set Search Paths ---
	// Define the name of the config file to look for (without extension).
	viper.SetConfigName("config")
	// Add paths where Viper should look for the config file.
	viper.AddConfigPath(".")                 // Current working directory
	viper.AddConfigPath("/etc/webcrawler/")  // System-wide configuration
	viper.AddConfigPath("$HOME/.webcrawler") // User-specific configuration

	// --- Set Defaults ---
	// Set sensible defaults for key configuration parameters. These will be used
	// if the values are not provided in a config file or via environment variables.
	const defaultUA = "RealtimeCPI-Crawler/1.0 (+http://github.com/JakeFAU/realtime-cpi)"
	viper.SetDefault("crawler.user_agent", defaultUA)
	viper.SetDefault("crawler.useragent", defaultUA) // backward compatibility
	viper.SetDefault("crawler.respect_robots", true)
	viper.SetDefault("crawler.concurrency", 8)
	viper.SetDefault("crawler.max_depth", 3)
	viper.SetDefault("crawler.blocked_domains", []string{})
	viper.SetDefault("crawler.target_urls", []string{"https://www.google.com"})
	viper.SetDefault("crawler.delay_seconds", 2)
	viper.SetDefault("crawler.ignore_robots", false)
	viper.SetDefault("crawler.rate_limit_backoff_seconds", 5)
	viper.SetDefault("crawler.max_forbidden_responses", 3)
	viper.SetDefault("http.timeout_seconds", 15)

	// Modern crawler pipeline defaults.
	viper.SetDefault("crawler.request_timeout", "10s")
	viper.SetDefault("crawler.rate_limit_per_domain", 2)
	viper.SetDefault("crawler.js_render_timeout", "15s")
	viper.SetDefault("crawler.js_render_max_concurrency", 2)
	viper.SetDefault("crawler.js_render_domain_qps", 0.5)
	viper.SetDefault("crawler.escalate_only_same_host", true)
	viper.SetDefault("crawler.feature_render_enabled", false)
	viper.SetDefault("crawler.max_page_bytes", 5*1024*1024)
	viper.SetDefault("crawler.output_dir", "data/crawl")

	viper.SetDefault("detector.min_html_bytes", 2000)
	viper.SetDefault("detector.selector_must", ".main,.app,.content")
	viper.SetDefault("detector.keywords", []string{
		"__NEXT_DATA__",
		"data-reactroot",
		"ng-app",
		"window.__APOLLO_STATE__",
	})

	// --- Environment Variables ---
	// Enable Viper to read environment variables.
	viper.SetEnvPrefix("CRAWLER") // e.g., CRAWLER_HTTP_TIMEOUT_SECONDS=30
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// --- Read Config File ---
	// Attempt to read the configuration file.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; this is not a fatal error if we can proceed
			// with defaults and environment variables.
			logging.L.Warn("Config file not found; using defaults and environment variables.")
		} else {
			// A real error occurred while parsing the config file.
			logging.L.Error("Error reading config file", zap.Error(err))
		}
	} else {
		logging.L.Info("Using config file", zap.String("path", viper.ConfigFileUsed()))
	}
}
