// Package main wires together the crawler service binaries.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/JakeFAU/realtime-cpi-crawler/internal/config"
	"github.com/JakeFAU/realtime-cpi-crawler/internal/server"
	"go.uber.org/zap"
)

func main() {
	cfgPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	app, err := server.Build(context.Background(), &cfg)
	if err != nil {
		// Assuming logger is not yet available, so using fmt.
		fmt.Fprintf(os.Stderr, "application build failed: %v\n", err)
		os.Exit(1)
	}

	if err := app.Run(context.Background()); err != nil {
		// The logger should be initialized within the app, but as a fallback.
		if logger, err := zap.NewProduction(); err == nil {
			logger.Fatal("application runtime error", zap.Error(err))
		} else {
			fmt.Fprintf(os.Stderr, "application runtime error: %v\n", err)
		}
		os.Exit(1)
	}
}
