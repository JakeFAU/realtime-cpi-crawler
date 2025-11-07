// Package app initializes and holds long-lived application services, acting as a dependency injection container.
package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/database"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/logging"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/queue"
	"github.com/JakeFAU/realtime-cpi/webcrawler/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// App holds all the shared, long-lived services for the application.
// It acts as a dependency injection (DI) container, holding instances of services
// like the logger, database, storage provider, and message queue.
// This struct is initialized once at startup and passed to the components that need it.
type App struct {
	logger   *zap.Logger
	storage  storage.Provider
	database database.Provider
	queue    queue.Provider
}

// GetLogger returns the shared zap logger instance for request-scoped logging.
func (a *App) GetLogger() *zap.Logger {
	return a.logger
}

// GetStorage exposes the configured blob storage provider.
func (a *App) GetStorage() storage.Provider {
	return a.storage
}

// GetDatabase provides access to the crawl metadata database provider.
func (a *App) GetDatabase() database.Provider {
	return a.database
}

// GetQueue returns the queue provider used to publish crawl notifications.
func (a *App) GetQueue() queue.Provider {
	return a.queue
}

// NewApp creates and initializes a new App struct based on the application's configuration.
// It is the central point for service initialization. It reads configuration values from Viper
// and instantiates the appropriate providers (e.g., GCS for storage, Postgres for database).
// This function is designed to fail fast if any critical service cannot be initialized.
func NewApp(ctx context.Context) (*App, error) {
	l := logging.L // Get the global logger
	l.Info("Initializing application services...")

	// 1. Initialize Storage Provider
	// The storage provider is responsible for saving the raw HTML content of crawled pages.
	storageProviderType := viper.GetString("storage.provider")
	var store storage.Provider
	var err error

	switch storageProviderType {
	case "gcs":
		bucketName := viper.GetString("storage.gcs.bucket_name")
		if bucketName == "" {
			return nil, fmt.Errorf("storage provider is 'gcs' but storage.gcs.bucket_name is not set")
		}
		l.Info("Using GCS storage provider", zap.String("bucket", bucketName))
		store, err = storage.NewGCSProvider(ctx, bucketName, &storage.DefaultGCSClientFactory{})
	case "noop":
		l.Info("Using No-Op storage provider. HTML content will be discarded.")
		store = &storage.NoOpProvider{}
	default:
		return nil, fmt.Errorf("unknown storage provider: %s", storageProviderType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// 2. Initialize Database Provider
	// The database provider is used to store metadata about each crawl.
	dbProviderType := viper.GetString("database.provider")
	var db database.Provider
	switch dbProviderType {
	case "postgres":
		dsn := viper.GetString("database.postgres.dsn")
		if dsn == "" {
			return nil, fmt.Errorf("database provider is 'postgres' but database.postgres.dsn is not set")
		}
		l.Info("Connecting to PostgreSQL...")
		db, err = database.NewPostgresProvider(ctx, dsn, &database.SQLXConnector{})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	case "noop":
		l.Info("Using No-Op database provider. Metadata will be discarded.")
		db = &database.NoOpProvider{}
	default:
		return nil, fmt.Errorf("unknown database provider: %s", dbProviderType)
	}

	// 3. Initialize Queue Provider
	// The queue provider is used to send notifications about completed crawls.
	queueProviderType := viper.GetString("queue.provider")
	var q queue.Provider
	switch queueProviderType {
	case "pubsub":
		projectID := viper.GetString("queue.gcp.project_id")
		topicID := viper.GetString("queue.gcp.topic_id")
		if projectID == "" || topicID == "" {
			return nil, fmt.Errorf("queue provider is 'pubsub' but project_id or topic_id is not set")
		}
		l.Info("Connecting to GCP Pub/Sub", zap.String("topic", topicID))
		q, err = queue.NewPubSubProvider(ctx, projectID, topicID, &queue.DefaultClientFactory{})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize queue: %w", err)
		}
	case "noop":
		l.Info("Using No-Op queue provider. No messages will be sent.")
		q = &queue.NoOpProvider{}
	default:
		return nil, fmt.Errorf("unknown queue provider: %s", queueProviderType)
	}

	l.Info("Application services initialized successfully.")

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		l.Info("Starting metrics server on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			l.Error("Metrics server failed", zap.Error(err))
		}
	}()

	return &App{
		logger:   l,
		storage:  store,
		database: db,
		queue:    q,
	}, nil
}

// Close gracefully shuts down all services in the App container.
// It is called by a Cobra hook after the command finishes execution.
func (a *App) Close() {
	a.GetLogger().Info("Shutting down application services...")
	if err := a.GetDatabase().Close(); err != nil {
		a.GetLogger().Warn("Error closing database connection", zap.Error(err))
	}
	if err := a.GetQueue().Close(); err != nil {
		a.GetLogger().Warn("Error closing queue client", zap.Error(err))
	}
	// Note: The GCS storage client does not require an explicit close operation.

	// Flushing the logger buffer is important to ensure all logs are written before the application exits.
	if err := a.GetLogger().Sync(); err != nil {
		// We can't do much here, as logging itself might be failing.
		// This is a best-effort attempt.
		a.GetLogger().Warn("Error syncing logger on shutdown", zap.Error(err))
	}
}
