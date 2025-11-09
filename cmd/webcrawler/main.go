// Package main wires together the crawler service binaries.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	memorypublisher "github.com/JakeFAU/realtime-cpi-crawler/internal/publisher/memory"
	queueMemory "github.com/JakeFAU/realtime-cpi-crawler/internal/queue/memory"
	memoryStorage "github.com/JakeFAU/realtime-cpi-crawler/internal/storage/memory"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/worker"
)

func main() {
	cfgPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		return
	}

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
			logger.Error("headless fetcher init failed", "error", err)
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
		logger.Info("http server started", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	queue.Close()
	logger.Info("shutdown complete")
}
