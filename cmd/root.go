package cmd

import (
	"context"
	"fmt"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/app"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/JakeFAU/realtime-cpi/webcrawler/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cfgFile string

// appKeyType is the key for storing the App in the context.
type appKeyType string

const appKey appKeyType = "app"

// App defines the application interface that commands will use.
// This allows us to inject a mock app during tests.
type App interface {
	Close()
	GetLogger() *zap.Logger
	GetStorage() storage.Provider
	GetDatabase() database.Provider
	GetQueue() queue.Provider
}

// newApp is the application factory. It's a variable so we can
// replace it with a mock factory in our tests.
var newApp func(ctx context.Context) (App, error) = func(ctx context.Context) (App, error) {
	// This assumes your *app.App satisfies the App interface.
	// You will need to add the GetLogger(), GetStorage(), etc.
	// methods to your *app.App struct.
	return app.NewApp(ctx)
}

// newRootCmd creates and configures the root command.
// All logic from init() has been moved here.
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webcrawler",
		Short: "A high-concurrency web crawler for the Realtime CPI project.",
		Long: `webcrawler is the primary ingestion tool for the Realtime CPI project.
It fetches product pages from across the web at scale, using a distributed
and resilient architecture to handle millions of requests.`,

		// This hook runs AFTER config is loaded but BEFORE the subcommand's RunE.
		// This is the perfect place to build and inject the application.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			appInstance, err := newApp(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to initialize application services: %w", err)
			}

			// Store the app instance in the context for subcommands to use.
			ctx := context.WithValue(cmd.Context(), appKey, appInstance)
			cmd.SetContext(ctx)
			return nil
		},

		// This hook ensures services are shut down gracefully.
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			// Retrieve the app INTERFACE from the context and close it.
			if appInstance, ok := cmd.Context().Value(appKey).(App); ok && appInstance != nil {
				appInstance.Close()
			}
		},
	}

	// Initialize Viper configuration.
	cobra.OnInitialize(config.InitConfig)

	// Define persistent flags.
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.webcrawler.yaml)")

	// Add subcommands. They no longer take the app as an argument.
	cmd.AddCommand(newCrawlCmd())

	return cmd
}

// Execute is the main entry point.
func Execute() {
	// Initialize the logger once at the very start.
	logging.InitLogger()

	// Create and execute the root command.
	if err := newRootCmd().Execute(); err != nil {
		logging.L.Fatal("Command execution failed", zap.Error(err))
	}
}
