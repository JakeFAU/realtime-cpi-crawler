// Package cmd defines and implements the CLI commands for the webcrawler executable.
package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// newCrawlCmd creates and configures the 'crawl' subcommand.
// This command is responsible for initiating the web crawling process based on the application's configuration.
// It retrieves the application instance from the context and uses it to create and run the crawler.
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

			cfg, err := crawler.LoadCrawlerConfig(viper.GetViper())
			if err != nil {
				return fmt.Errorf("load crawler config: %w", err)
			}

			logger := appInstance.GetLogger()

			fetcher, err := crawler.NewCollyFetcher(cfg, logger)
			if err != nil {
				return fmt.Errorf("init fetcher: %w", err)
			}

			var renderer crawler.Renderer
			if cfg.FeatureRenderEnabled && cfg.JSRenderMaxConcurrency > 0 {
				renderer, err = crawler.NewChromedpRenderer(cfg, logger)
				if err != nil && !errors.Is(err, crawler.ErrRendererDisabled) {
					return fmt.Errorf("init renderer: %w", err)
				}
				if errors.Is(err, crawler.ErrRendererDisabled) {
					logger.Warn("Renderer disabled despite feature flag; falling back to fast path")
				}
			}

			detector := crawler.NewHeuristicDetector(cfg.DetectorMinHTMLBytes, cfg.DetectorSelectorMust, cfg.DetectorKeywords)
			robots := crawler.NewRobotsEnforcer(cfg.RespectRobots, cfg.UserAgent, logger)
			sink, err := crawler.NewFileSystemSink(cfg.OutputDir, cfg.MaxPageBytes, logger)
			if err != nil {
				return fmt.Errorf("init sink: %w", err)
			}

			engine := crawler.NewEngine(cfg, fetcher, renderer, detector, sink, robots, crawler.NewExponentialRetryPolicy(), logger)
			defer func() {
				if err := engine.Close(cmd.Context()); err != nil {
					logger.Warn("Failed to close engine", zap.Error(err))
				}
			}()

			if err := engine.Run(cmd.Context()); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}

			logging.L.Info("Crawl command finished.")
			return nil
		},
	}
	return cmd
}
