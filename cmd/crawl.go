// Package cmd defines and implements the CLI commands for the webcrawler executable.
package cmd

import (
	"context"
	"time"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/app"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newCrawlCmd creates the `crawl` subcommand.
// It accepts the *app.App as an argument, following a dependency injection pattern.
func newCrawlCmd(app *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "Starts the web crawler",
		Long: `Initiates a concurrent web crawl based on the URLs and settings
provided in the configuration file. This command uses the Colly framework
to perform the crawl.`,
		Run: func(_ *cobra.Command, _ []string) {
			// 1. Create Crawler Config from Viper
			// This command's responsibility is to translate configuration from Viper
			// into the concrete crawler.Config struct.
			cfg := crawler.Config{
				AllowedDomains:    viper.GetStringSlice("crawler.allowed_domains"),
				UserAgent:         viper.GetString("crawler.useragent"),
				HTTPTimeout:       time.Duration(viper.GetInt("http.timeout_seconds")) * time.Second,
				MaxDepth:          viper.GetInt("crawler.max_depth"),
				InitialTargetURLs: viper.GetStringSlice("crawler.target_urls"),
				Concurrency:       viper.GetInt("crawler.concurrency"),
				Delay:             time.Duration(viper.GetInt("crawler.delay_seconds")) * time.Second,
			}

			// 2. Create and Run the Crawler
			// We instantiate the new Colly-based crawler, passing in the configuration
			// and the application services from the App struct.
			c := crawler.NewCollyCrawler(cfg, app.Logger, app.Storage, app.Database, app.Queue)
			c.Run(context.Background()) // Start the crawl.

			logging.L.Info("Crawl command finished.")
		},
	}
	return cmd
}
