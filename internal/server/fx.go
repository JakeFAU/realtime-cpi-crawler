// Package server provides the core application server and dependency injection.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/api"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/clock/system"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/crawler"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/dispatcher"
	collyfetcher "github.com/JakeFAU/realtime-cpi-crawler/internal/fetcher/colly"
	headlessfetcher "github.com/JakeFAU/realtime-cpi-crawler/internal/fetcher/headless"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/hash/sha256"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/headless/detector"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/id/uuid"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/logging"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/metrics"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
	progresssinks "github.com/JakeFAU/realtime-cpi-crawler/internal/progress/sinks"
	memorypublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/memory"
	gcppublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/pubsub"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
	gcsstorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/gcs"
	memoryStorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/memory"
	pgstore "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/postgres"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
	"go.uber.org/zap"
)

// App contains the application's dependencies.
type App struct {
	cfg            *config.Config
	logger         *zap.Logger
	apiServer      *api.Server
	dispatch       *dispatcher.Dispatcher
	progressHub    *progress.Hub
	queue          *queueMemory.Queue
	pubsubClient   *pubsub.Client
	pubsubTopic    *pubsub.Topic
	storage        *storage.Client
	retrievalStore crawler.RetrievalStore
	progressRepo   store.ProgressRepository
}

// NewApp creates a new App with the given configuration.
func NewApp(cfg *config.Config, logger *zap.Logger) (*App, error) {
	return &App{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Run starts the application and blocks until the context is canceled.
func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		a.logger.Info("dispatcher started")
		a.dispatch.Run(ctx)
	}()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", a.cfg.Server.Port),
		Handler:           a.apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		a.logger.Info("http server started", zap.Int("port", a.cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("http server error", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	a.logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		a.logger.Error("server shutdown error", zap.Error(err))
	}

	return a.Close(shutdownCtx)
}

// Close gracefully shuts down the application.
//
//nolint:gocognit
func (a *App) Close(ctx context.Context) error {
	a.queue.Close()
	if a.progressHub != nil {
		if err := a.progressHub.Close(ctx); err != nil {
			a.logger.Warn("progress hub close failed", zap.Error(err))
		}
	}
	if a.pubsubTopic != nil {
		a.pubsubTopic.Stop()
	}
	if a.pubsubClient != nil {
		if err := a.pubsubClient.Close(); err != nil {
			a.logger.Warn("pubsub client close failed", zap.Error(err))
		}
	}
	if a.storage != nil {
		if err := a.storage.Close(); err != nil {
			a.logger.Warn("gcs client close failed", zap.Error(err))
		}
	}
	if a.retrievalStore != nil {
		if err := a.retrievalStore.Close(); err != nil {
			a.logger.Warn("retrieval store close failed", zap.Error(err))
		}
	}
	if a.progressRepo != nil {
		if pgRepo, ok := a.progressRepo.(*pgstore.ProgressStore); ok {
			pgRepo.Close()
		}
	}
	if err := a.logger.Sync(); err != nil {
		a.logger.Warn("logger sync failed", zap.Error(err))
	}
	a.logger.Info("shutdown complete")
	return nil
}

// Build creates the application's dependencies.
func Build(ctx context.Context, cfg *config.Config) (*App, error) {
	logger, err := logging.New(cfg.Logging.Development)
	if err != nil {
		return nil, fmt.Errorf("logger init failed: %w", err)
	}
	zap.ReplaceGlobals(logger)

	app, err := NewApp(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("app init failed: %w", err)
	}

	jobStore := memoryStorage.NewJobStore()

	blobStore, err := setupStorage(ctx, app)
	if err != nil {
		return nil, err
	}

	if err = setupDatabase(ctx, app); err != nil {
		return nil, err
	}

	publisher, err := setupPublisher(ctx, app)
	if err != nil {
		return nil, err
	}

	meters := metrics.New()
	progressEmitter, err := setupProgress(ctx, app, meters, app.progressRepo)
	if err != nil {
		return nil, err
	}

	app.queue = queueMemory.NewQueue(cfg.Crawler.GlobalQueueDepth).WithMetrics(meters)
	app.dispatch, err = setupDispatcher(app, jobStore, blobStore, publisher, progressEmitter, meters)
	if err != nil {
		return nil, err
	}

	app.apiServer = api.NewServer(
		jobStore,
		app.dispatch,
		uuid.NewUUIDGenerator(),
		system.New(),
		*cfg,
		meters,
		logger.Named("api"),
		app.progressRepo,
	)

	return app, nil
}

func setupStorage(ctx context.Context, app *App) (crawler.BlobStore, error) {
	var blobStore crawler.BlobStore
	var err error
	switch app.cfg.Storage.Backend {
	case "gcs":
		app.storage, err = storage.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("gcs client init failed: %w", err)
		}
		blobStore, err = gcsstorage.New(app.storage, gcsstorage.Config{
			Bucket: app.cfg.Storage.Bucket,
		})
		if err != nil {
			return nil, fmt.Errorf("gcs blob store init failed: %w", err)
		}
	default:
		blobStore = memoryStorage.NewBlobStore()
	}
	return blobStore, nil
}

func setupDatabase(ctx context.Context, app *App) error {
	if app.cfg.Database.DSN == "" {
		return nil
	}
	var err error
	app.retrievalStore, err = pgstore.NewRetrievalStore(ctx, pgstore.RetrievalStoreConfig{
		DSN:             app.cfg.Database.DSN,
		Table:           app.cfg.Database.Table,
		MaxConns:        app.cfg.Database.MaxConns,
		MinConns:        app.cfg.Database.MinConns,
		MaxConnLifetime: app.cfg.Database.MaxConnLifetime,
	})
	if err != nil {
		return fmt.Errorf("retrieval store init failed: %w", err)
	}
	app.progressRepo, err = pgstore.NewProgressStore(ctx, app.cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("progress store init failed: %w", err)
	}
	return nil
}

func setupPublisher(ctx context.Context, app *App) (crawler.Publisher, error) {
	if app.cfg.PubSub.TopicName == "" || app.cfg.PubSub.ProjectID == "" {
		return memorypublisher.New(), nil
	}
	var err error
	app.pubsubClient, err = pubsub.NewClient(ctx, app.cfg.PubSub.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub client init failed: %w", err)
	}
	app.pubsubTopic = app.pubsubClient.Topic(app.cfg.PubSub.TopicName)
	return gcppublisher.New(app.pubsubTopic), nil
}

func setupProgress(
	ctx context.Context,
	app *App,
	meters *metrics.Collectors,
	progressRepo store.ProgressRepository,
) (progress.Emitter, error) {
	if !app.cfg.Progress.Enabled {
		return nil, nil
	}
	var sinkList []progress.Sink
	if meters != nil && meters.Registry() != nil {
		promSink, err := progresssinks.NewPrometheusSink(meters.Registry())
		if err != nil {
			app.logger.Warn("prometheus progress sink init failed", zap.Error(err))
		} else {
			sinkList = append(sinkList, promSink)
		}
	}
	if progressRepo != nil {
		sinkList = append(sinkList, progresssinks.NewStoreSink(progressRepo, app.logger.Named("progress_store")))
	}
	if app.cfg.Progress.LogEnabled {
		sinkList = append(sinkList, progresssinks.NewLogSink(app.logger.Named("progress_log")))
	}
	if len(sinkList) == 0 {
		return nil, nil
	}
	hubCfg := progress.Config{
		BufferSize:     app.cfg.Progress.BufferSize,
		MaxBatchEvents: app.cfg.Progress.Batch.MaxEvents,
		MaxBatchWait:   time.Duration(app.cfg.Progress.Batch.MaxWaitMs) * time.Millisecond,
		SinkTimeout:    time.Duration(app.cfg.Progress.SinkTimeoutMs) * time.Millisecond,
		BaseContext:    ctx,
		Logger:         app.logger.Named("progress_hub"),
	}
	app.progressHub = progress.NewHub(hubCfg, sinkList...)
	return app.progressHub, nil
}

func setupDispatcher(
	app *App,
	jobStore crawler.JobStore,
	blobStore crawler.BlobStore,
	publisher crawler.Publisher,
	progressEmitter progress.Emitter,
	meters *metrics.Collectors,
) (*dispatcher.Dispatcher, error) {
	hasher := sha256.New()
	clock := system.New()
	idGen := uuid.NewUUIDGenerator()
	detect := detector.NewHeuristic(app.cfg.Headless.PromotionThresh)
	probeFetcher := collyfetcher.New(collyfetcher.Config{
		UserAgent:     app.cfg.Crawler.UserAgent,
		RespectRobots: !app.cfg.Crawler.IgnoreRobots,
		Timeout:       time.Duration(app.cfg.HTTP.TimeoutSeconds) * time.Second,
	})
	var headless crawler.Fetcher
	if app.cfg.Headless.Enabled {
		var err error
		headless, err = headlessfetcher.NewChromedp(headlessfetcher.Config{
			MaxParallel:       app.cfg.Headless.MaxParallel,
			UserAgent:         app.cfg.Crawler.UserAgent,
			NavigationTimeout: time.Duration(app.cfg.Headless.NavTimeoutSec) * time.Millisecond,
		})
		if err != nil {
			app.logger.Warn("headless fetcher init failed", zap.Error(err))
		}
	}

	workerCfg := worker.Config{
		ContentType: app.cfg.Storage.ContentType,
		BlobPrefix:  app.cfg.Storage.Prefix,
		Topic:       app.cfg.PubSub.TopicName,
	}

	var workers []*worker.Worker
	for i := 0; i < app.cfg.Crawler.Concurrency; i++ {
		workers = append(workers, worker.New(
			app.queue,
			jobStore,
			blobStore,
			app.retrievalStore,
			publisher,
			hasher,
			clock,
			probeFetcher,
			headless,
			detect,
			nil,
			idGen,
			progressEmitter,
			workerCfg,
			app.logger.Named("worker").With(zap.Int("index", i)),
			meters,
		))
	}
	return dispatcher.New(app.queue, workers), nil
}
