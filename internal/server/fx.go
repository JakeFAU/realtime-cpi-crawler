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

	pubsub "cloud.google.com/go/pubsub/v2"
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
	"github.com/JakeFAU/realtime-cpi-crawler/internal/policy/ratelimit"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/policy/simple"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/progress"
	progresssinks "github.com/JakeFAU/realtime-cpi-crawler/internal/progress/sinks"
	memorypublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/memory"
	gcppublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/pubsub"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
	gcsstorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/gcs"
	localstorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/local"
	memoryStorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/memory"
	pgstore "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/postgres"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/store"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/telemetry"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
	"go.uber.org/zap"
)

// App contains the application's dependencies.
type App struct {
	cfg             *config.Config
	logger          *zap.Logger
	apiServer       *api.Server
	dispatch        *dispatcher.Dispatcher
	progressHub     *progress.Hub
	queue           *queueMemory.Queue
	pubsubClient    *pubsub.Client
	pubsubPublisher *pubsub.Publisher
	storage         *storage.Client
	retrievalStore  crawler.RetrievalStore
	progressRepo    store.ProgressRepository
	tracerShutdown  func(context.Context) error
	metricShutdown  func(context.Context) error
}

// NewApp creates a new App with the given configuration.
func NewApp(cfg *config.Config, logger *zap.Logger) (*App, error) {
	// Define a struct for logging only non-sensitive config fields
	type SanitizedConfig struct {
		ServerPort  int    `json:"server_port"`
		Environment string `json:"environment,omitempty"`
	}
	safeCfg := SanitizedConfig{
		ServerPort: cfg.Server.Port,
	}
	logger.Info("Creating application", zap.Any("config", safeCfg))
	return &App{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Run starts the application and blocks until the context is canceled.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info("application started")
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
func (a *App) Close(ctx context.Context) error {
	a.queue.Close()
	a.closeInfrastructure(ctx)
	a.closeObservability(ctx)
	a.logger.Info("shutdown complete")
	return nil
}

//nolint:gocognit // Shutdown logic is linear but extensive, ignoring complexity check
func (a *App) closeInfrastructure(ctx context.Context) {
	if a.progressHub != nil {
		if err := a.progressHub.Close(ctx); err != nil {
			a.logger.Warn("progress hub close failed", zap.Error(err))
		}
	}
	if a.pubsubPublisher != nil {
		a.pubsubPublisher.Stop()
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
}

func (a *App) closeObservability(ctx context.Context) {
	if err := a.logger.Sync(); err != nil {
		a.logger.Warn("logger sync failed", zap.Error(err))
	}
	if a.tracerShutdown != nil {
		if err := a.tracerShutdown(ctx); err != nil {
			a.logger.Warn("tracer shutdown failed", zap.Error(err))
		}
	}
	if a.metricShutdown != nil {
		if err := a.metricShutdown(ctx); err != nil {
			a.logger.Warn("metric shutdown failed", zap.Error(err))
		}
	}
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

	// Initialize tracing
	tp, mp, err := telemetry.InitTelemetry(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("tracer init failed: %w", err)
	}
	app.tracerShutdown = tp.Shutdown
	app.metricShutdown = mp.Shutdown

	app.logger.Info("building application dependencies")
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

	progressEmitter, err := setupProgress(ctx, app, app.progressRepo)
	if err != nil {
		return nil, err
	}

	app.queue = queueMemory.NewQueue(cfg.Crawler.GlobalQueueDepth)
	app.dispatch, err = setupDispatcher(app, jobStore, blobStore, publisher, progressEmitter)
	if err != nil {
		return nil, err
	}

	app.apiServer = api.NewServer(
		jobStore,
		app.dispatch,
		uuid.NewUUIDGenerator(),
		system.New(),
		*cfg,
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
		app.logger.Info("using GCS storage backend")
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
		app.logger.Debug("GCS storage backend", zap.String("bucket", app.cfg.Storage.Bucket))
	case "local":
		app.logger.Info("using local storage backend")
		blobStore, err = localstorage.New(app.cfg.Storage.Local)
		if err != nil {
			return nil, fmt.Errorf("local blob store init failed: %w", err)
		}
		app.logger.Debug("local storage backend", zap.String("path", app.cfg.Storage.Local.BaseDir))
	default:
		app.logger.Info("using in-memory storage backend")
		blobStore = memoryStorage.NewBlobStore()
	}
	return blobStore, nil
}

func setupDatabase(ctx context.Context, app *App) error {
	if app.cfg.Database.DSN == "" {
		app.logger.Warn("No DSN specified for database, skipping retrieval store and progress repository initialization")
		return nil
	}
	var err error
	app.retrievalStore, err = pgstore.NewRetrievalStore(ctx, pgstore.RetrievalStoreConfig{
		DSN:             app.cfg.Database.DSN,
		Table:           app.cfg.Database.RetrievalTable,
		ProgressTable:   app.cfg.Database.ProgressTable,
		MaxConns:        app.cfg.Database.MaxConns,
		MinConns:        app.cfg.Database.MinConns,
		MaxConnLifetime: app.cfg.Database.MaxConnLifetime,
	})
	if err != nil {
		return fmt.Errorf("retrieval store init failed: %w", err)
	}
	app.logger.Info("retrieval store initialized", zap.String("table", app.cfg.Database.RetrievalTable))
	app.progressRepo, err = pgstore.NewProgressStore(ctx, app.cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("progress store init failed: %w", err)
	}
	return nil
}

func setupPublisher(ctx context.Context, app *App) (crawler.Publisher, error) {
	if app.cfg.PubSub.TopicName == "" || app.cfg.PubSub.ProjectID == "" {
		app.logger.Warn("No Pub/Sub topic configured, using in-memory publisher")
		return memorypublisher.New(), nil
	}
	var err error
	app.pubsubClient, err = pubsub.NewClient(ctx, app.cfg.PubSub.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("pubsub client init failed: %w", err)
	}
	app.pubsubPublisher = app.pubsubClient.Publisher(app.cfg.PubSub.TopicName)
	app.logger.Info(
		"Pub/Sub publisher initialized",
		zap.String("project", app.cfg.PubSub.ProjectID),
		zap.String("topic", app.cfg.PubSub.TopicName),
	)
	return gcppublisher.New(app.pubsubPublisher), nil
}

func setupProgress(
	ctx context.Context,
	app *App,
	progressRepo store.ProgressRepository,
) (progress.Emitter, error) {
	if !app.cfg.Progress.Enabled {
		app.logger.Info("progress tracking disabled")
		return nil, nil
	}
	var sinkList []progress.Sink
	if progressRepo != nil {
		sinkList = append(
			sinkList,
			progresssinks.NewStoreSink(progressRepo, app.logger.Named("progress_store")),
		)
		app.logger.Debug("Added progress store sink")
	}
	if app.cfg.Progress.LogEnabled {
		sinkList = append(
			sinkList,
			progresssinks.NewLogSink(app.logger.Named("progress_log")),
		)
		app.logger.Debug("Added progress log sink")
	}
	if len(sinkList) == 0 {
		app.logger.Warn("progress tracking enabled but no sinks configured")
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
	app.logger.Info("progress hub initialized",
		zap.Int("buffer_size", hubCfg.BufferSize),
		zap.Int("max_batch_events", hubCfg.MaxBatchEvents),
		zap.Duration("max_batch_wait", hubCfg.MaxBatchWait),
		zap.Duration("sink_timeout", hubCfg.SinkTimeout),
	)
	return app.progressHub, nil
}

func setupDispatcher(
	app *App,
	jobStore crawler.JobStore,
	blobStore crawler.BlobStore,
	publisher crawler.Publisher,
	progressEmitter progress.Emitter,
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
	app.logger.Info("using colly probe fetcher", zap.String("user_agent", app.cfg.Crawler.UserAgent))
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
		app.logger.Info("using headless fetcher", zap.Int("max_parallel", app.cfg.Headless.MaxParallel))
	}

	workerCfg := worker.Config{
		ContentType:    app.cfg.Storage.ContentType,
		BlobPrefix:     app.cfg.Storage.Prefix,
		Topic:          app.cfg.PubSub.TopicName,
		JobTimeout:     app.cfg.JobBudget(),
		RequestTimeout: time.Duration(app.cfg.HTTP.TimeoutSeconds) * time.Second,
	}
	app.logger.Info("worker config",
		zap.String("content_type", workerCfg.ContentType),
		zap.String("blob_prefix", workerCfg.BlobPrefix),
		zap.String("topic", workerCfg.Topic),
		zap.Duration("job_timeout", workerCfg.JobTimeout),
		zap.Duration("request_timeout", workerCfg.RequestTimeout),
	)

	var policy crawler.Policy
	if app.cfg.RateLimit.Enabled {
		policy = ratelimit.New(ratelimit.Config{
			DefaultRPS:   app.cfg.RateLimit.DefaultRPS,
			DefaultBurst: app.cfg.RateLimit.DefaultBurst,
		})
		app.logger.Info("rate limiter enabled",
			zap.Float64("default_rps", app.cfg.RateLimit.DefaultRPS),
			zap.Int("default_burst", app.cfg.RateLimit.DefaultBurst),
		)
	} else {
		policy = simple.New()
		app.logger.Info("rate limiter disabled, using simple policy")
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
			policy,
			idGen,
			progressEmitter,
			workerCfg,
			app.logger.Named("worker").With(zap.Int("index", i)),
		))
	}
	return dispatcher.New(app.queue, workers), nil
}
