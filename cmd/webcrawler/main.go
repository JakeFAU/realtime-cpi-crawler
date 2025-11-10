// Package main wires together the crawler service binaries.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"

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
	memorypublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/memory"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
	gcsstorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/gcs"
	memoryStorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/memory"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
)

//gocognit:ignore
func main() {
	cfgPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}
	logger, err := logging.New(cfg.Logging.Development)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			fmt.Fprintf(os.Stderr, "logger sync failed: %v\n", syncErr)
		}
	}()
	zap.ReplaceGlobals(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	jobStore := memoryStorage.NewJobStore()
	var blobStore crawler.BlobStore
	var storageClient *storage.Client
	switch cfg.Storage.Backend {
	case "gcs":
		storageClient, err = storage.NewClient(ctx)
		if err != nil {
			logger.Fatal("gcs client init failed", zap.Error(err))
		}
		defer func() {
			if closeErr := storageClient.Close(); closeErr != nil {
				logger.Warn("gcs client close failed", zap.Error(closeErr))
			}
		}()
		blobStore, err = gcsstorage.New(storageClient, gcsstorage.Config{
			Bucket: cfg.Storage.Bucket,
		})
		if err != nil {
			logger.Fatal("gcs blob store init failed", zap.Error(err))
		}
	default:
		blobStore = memoryStorage.NewBlobStore()
	}
	publisher := memorypublisher.New()
	meters := metrics.New()
	queue := queueMemory.NewQueue(cfg.Crawler.GlobalQueueDepth).WithMetrics(meters)
	hasher := sha256.New()
	clock := system.New()
	idGen := uuid.NewUUIDGenerator()
	detect := detector.NewHeuristic(cfg.Headless.PromotionThresh)
	probeFetcher := collyfetcher.New(collyfetcher.Config{
		UserAgent:     cfg.Crawler.UserAgent,
		RespectRobots: !cfg.Crawler.IgnoreRobots,
		Timeout:       time.Duration(cfg.HTTP.TimeoutSeconds) * time.Second,
	})
	var headless crawler.Fetcher
	if cfg.Headless.Enabled {
		headlessFetcher, err := headlessfetcher.NewChromedp(headlessfetcher.Config{
			MaxParallel:       cfg.Headless.MaxParallel,
			UserAgent:         cfg.Crawler.UserAgent,
			NavigationTimeout: time.Duration(cfg.Headless.NavTimeoutSec) * time.Second,
		})
		if err != nil {
			logger.Warn("headless fetcher init failed", zap.Error(err))
		} else {
			headless = headlessFetcher
		}
	}

	workerCfg := worker.Config{
		ContentType: cfg.Storage.ContentType,
		BlobPrefix:  cfg.Storage.Prefix,
		Topic:       cfg.PubSub.TopicName,
	}

	var workers []*worker.Worker
	for i := 0; i < cfg.Crawler.Concurrency; i++ {
		workers = append(workers, worker.New(
			queue,
			jobStore,
			blobStore,
			publisher,
			hasher,
			clock,
			probeFetcher,
			headless,
			detect,
			nil,
			idGen,
			workerCfg,
			logger.Named("worker").With(zap.Int("index", i)),
			meters,
		))
	}
	dispatch := dispatcher.New(queue, workers)

	apiServer := api.NewServer(jobStore, dispatch, idGen, clock, cfg, meters, logger.Named("api"))

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("dispatcher started")
		dispatch.Run(ctx)
	}()

	go func() {
		logger.Info("http server started", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
	queue.Close()
	logger.Info("shutdown complete")
}
