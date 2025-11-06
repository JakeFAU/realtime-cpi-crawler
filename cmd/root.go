package cmd

import (
	"context"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/app"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/JakeFAU/realtime-cpi/webcrawler/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands.
// It is the main entry point for the webcrawler application.
var rootCmd = &cobra.Command{
	Use:   "webcrawler",
	Short: "A high-concurrency web crawler for the Realtime CPI project.",
	Long: `webcrawler is the primary ingestion tool for the Realtime CPI project.
It fetches product pages from across the web at scale, using a distributed
and resilient architecture to handle millions of requests.`,
	// This hook ensures that long-lived services are shut down gracefully
	// after any command has finished its execution.
	PersistentPostRun: func(cmd *cobra.Command, _ []string) {
		// Retrieve the app instance from the command's context and close it.
		if appInstance, ok := cmd.Context().Value("app").(*app.App); ok {
			appInstance.Close()
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	// Initialize the structured logger first, as other parts of the application
	// depend on it for logging during startup.
	logging.InitLogger()

	// Initialize Viper configuration. This function sets up default values,
	// environment variable bindings, and searches for a configuration file.
	cobra.OnInitialize(config.InitConfig)

	// Define a persistent flag for the configuration file path.
	// This allows users to specify a custom config file via the command line.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.webcrawler.yaml)")

	// Initialize the core application services (database, storage, queue, etc.).
	// This is done after the logger and config are ready.
	ctx := context.Background()
	appInstance, err := app.NewApp(ctx)
	if err != nil {
		logging.L.Fatal("Failed to initialize application services", zap.Error(err))
	}

	// Store the initialized app instance in the root command's context.
	// This makes it available to subcommands and the PersistentPostRun hook.
	type appKeyType string
	const appKey appKeyType = "app"
	ctx = context.WithValue(ctx, appKey, appInstance)
	rootCmd.SetContext(ctx)

	// Add subcommands to the root command.
	// We pass the initialized app struct to the subcommands, following a
	// dependency injection pattern.
	rootCmd.AddCommand(newCrawlCmd(appInstance))
}
