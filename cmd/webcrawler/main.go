// Package main wires together the crawler service binaries.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
<<<<<<< HEAD
=======
	"log/slog"
>>>>>>> b22344a4 (refactor to server)
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

<<<<<<< HEAD
	"go.uber.org/zap"

=======
>>>>>>> b22344a4 (refactor to server)
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
<<<<<<< HEAD
	"github.com/JakeFAU/realtime-cpi-crawler/internal/logging"
=======
>>>>>>> b22344a4 (refactor to server)
	memorypublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/memory"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
	memoryStorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/memory"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
)

func main() {
	cfgPath := flag.String("config", "", "Path to config file")
	flag.Parse()

<<<<<<< HEAD
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

=======
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		return
	}

>>>>>>> b22344a4 (refactor to server)
	jobStore := memoryStorage.NewJobStore()
	blobStore := memoryStorage.NewBlobStore()
	publisher := memorypublisher.New()
	queue := queueMemory.NewQueue(cfg.Crawler.GlobalQueueDepth)
	hasher := sha256.New()
	clock := system.New()
	idGen := uuid.New()
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
<<<<<<< HEAD
			logger.Warn("headless fetcher init failed", zap.Error(err))
=======
			logger.Error("headless fetcher init failed", "error", err)
>>>>>>> b22344a4 (refactor to server)
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
<<<<<<< HEAD
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
			workerCfg,
			logger.Named("worker").With(zap.Int("index", i)),
		))
	}
	dispatch := dispatcher.New(queue, workers)

	apiServer := api.NewServer(jobStore, dispatch, idGen, clock, cfg, logger.Named("api"))
=======
		workers = append(
			workers,
			worker.New(
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
				workerCfg,
				logger,
			),
		)
	}
	dispatch := dispatcher.New(queue, workers)

	apiServer := api.NewServer(jobStore, dispatch, idGen, clock, cfg)
>>>>>>> b22344a4 (refactor to server)

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
<<<<<<< HEAD
		logger.Info("http server started", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", zap.Error(err))
=======
		logger.Info("http server started", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
>>>>>>> b22344a4 (refactor to server)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
<<<<<<< HEAD
		logger.Error("server shutdown error", zap.Error(err))
=======
		logger.Error("server shutdown error", "error", err)
>>>>>>> b22344a4 (refactor to server)
	}
	queue.Close()
	logger.Info("shutdown complete")
}
