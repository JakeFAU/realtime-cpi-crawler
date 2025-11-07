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

		RunE: runCrawlCommand,
	}
	return cmd
}

func runCrawlCommand(cmd *cobra.Command, _ []string) error {
	appInstance, err := resolveApp(cmd.Context())
	if err != nil {
		return err
	}

	cfg, err := crawler.LoadCrawlerConfig(viper.GetViper())
	if err != nil {
		return fmt.Errorf("load crawler config: %w", err)
	}

	engine, err := buildCrawlerEngine(cfg, appInstance.GetLogger())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := engine.Close(cmd.Context()); cerr != nil {
			appInstance.GetLogger().Warn("Failed to close engine", zap.Error(cerr))
		}
	}()

	if err := engine.Run(cmd.Context()); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run crawler: %w", err)
	}

	logging.L.Info("Crawl command finished.")
	return nil
}

func resolveApp(ctx context.Context) (App, error) {
	appInstance, ok := ctx.Value(appKey).(App)
	if !ok || appInstance == nil {
		return nil, errors.New("application services not initialized")
	}
	return appInstance, nil
}

func buildCrawlerEngine(cfg crawler.Config, logger *zap.Logger) (*crawler.Engine, error) {
	fetcher, err := crawler.NewCollyFetcher(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init fetcher: %w", err)
	}

	renderer, err := buildRenderer(cfg, logger)
	if err != nil {
		return nil, err
	}

	detector := crawler.NewHeuristicDetector(
		cfg.DetectorMinHTMLBytes,
		cfg.DetectorSelectorMust,
		cfg.DetectorKeywords,
	)
	robots := crawler.NewRobotsEnforcer(cfg.RespectRobots, cfg.UserAgent, logger)
	sink, err := crawler.NewFileSystemSink(cfg.OutputDir, cfg.MaxPageBytes, logger)
	if err != nil {
		return nil, fmt.Errorf("init sink: %w", err)
	}

	engine := crawler.NewEngine(
		cfg,
		fetcher,
		renderer,
		detector,
		sink,
		robots,
		crawler.NewExponentialRetryPolicy(),
		logger,
	)
	return engine, nil
}

func buildRenderer(cfg crawler.Config, logger *zap.Logger) (crawler.Renderer, error) {
	if !cfg.FeatureRenderEnabled || cfg.JSRenderMaxConcurrency <= 0 {
		return nil, nil
	}
	renderer, err := crawler.NewChromedpRenderer(cfg, logger)
	switch {
	case err == nil:
		return renderer, nil
	case errors.Is(err, crawler.ErrRendererDisabled):
		logger.Warn("Renderer disabled despite feature flag; falling back to fast path")
		return nil, nil
	default:
		return nil, fmt.Errorf("init renderer: %w", err)
	}
}
