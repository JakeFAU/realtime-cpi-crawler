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

// InitConfig initializes Viper to read configuration from files and environment variables.
// This function is called once at application startup.
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
	viper.SetDefault("crawler.useragent", "RealtimeCPI-Crawler/1.0 (+http://github.com/JakeFAU/realtime-cpi)")
	viper.SetDefault("crawler.concurrency", 10)
	viper.SetDefault("crawler.max_depth", 1)
	viper.SetDefault("crawler.allowed_domains", []string{"www.google.com"})
	viper.SetDefault("crawler.target_urls", []string{"https://www.google.com"})
	viper.SetDefault("crawler.delay_seconds", 2)
	viper.SetDefault("http.timeout_seconds", 15)

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
