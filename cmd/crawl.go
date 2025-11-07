// Package cmd defines and implements the CLI commands for the webcrawler executable.
package cmd

import (
	"errors"
	"time"

	// No longer needs the 'app' package
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newCrawlCmd creates the `crawl` subcommand.
// It no longer accepts any arguments.
func newCrawlCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "Starts the web crawler",
		Long: `Initiates a concurrent web crawl based on the URLs and settings
provided in the configuration file. This command uses the Colly framework
to perform the crawl.`,

		// Use RunE to handle and return errors.
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 1. Retrieve the App interface from the context.
			appInstance, ok := cmd.Context().Value(appKey).(App)
			if !ok || appInstance == nil {
				return errors.New("application services not initialized")
			}

			// 2. Create Crawler Config from Viper (this part was correct)
			cfg := crawler.Config{
				AllowedDomains:        viper.GetStringSlice("crawler.allowed_domains"),
				UserAgent:             viper.GetString("crawler.useragent"),
				HTTPTimeout:           time.Duration(viper.GetInt("http.timeout_seconds")) * time.Second,
				MaxDepth:              viper.GetInt("crawler.max_depth"),
				InitialTargetURLs:     viper.GetStringSlice("crawler.target_urls"),
				Concurrency:           viper.GetInt("crawler.concurrency"),
				Delay:                 time.Duration(viper.GetInt("crawler.delay_seconds")) * time.Second,
				IgnoreRobots:          viper.GetBool("crawler.ignore_robots"),
				RateLimitBackoff:      time.Duration(viper.GetInt("crawler.rate_limit_backoff_seconds")) * time.Second,
				MaxForbiddenResponses: viper.GetInt("crawler.max_forbidden_responses"),
			}

			// 3. Create and Run the Crawler
			// Use the getter methods from the App interface.
			c := crawler.NewCollyCrawler(
				cfg,
				appInstance.GetLogger(),
				appInstance.GetStorage(),
				appInstance.GetDatabase(),
				appInstance.GetQueue(),
			)

			// Use the command's context so the crawl can be cancelled.
			c.Run(cmd.Context())

			logging.L.Info("Crawl command finished.")
			return nil
		},
	}
	return cmd
}
