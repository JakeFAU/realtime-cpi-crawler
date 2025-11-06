// Package logging configures the global structured logger for the application.
// It uses Uber's Zap library for high-performance, structured logging.
package logging

import (
	"log"

	"go.uber.org/zap"
)

// L is the global, structured logger instance used throughout the application.
// It is initialized by the InitLogger function.
var L *zap.Logger

// InitLogger initializes the global Zap logger.
// It is called once from the root command's init function, ensuring that the
// logger is available from the very start of the application's lifecycle.
func InitLogger() {
	// For production, we would typically use zap.NewProduction().
	// However, zap.NewDevelopment() provides more human-readable output with
	// color-coded levels, which is ideal for development and debugging.
	// This could be made configurable based on an environment variable.
	logger, err := zap.NewDevelopment()
	if err != nil {
		// If Zap fails to initialize, we fall back to the standard Go logger.
		log.Fatalf("Failed to initialize Zap logger: %v", err)
	}

	// Set the global logger instance for direct access.
	L = logger

	// Replace the global default Zap logger. This allows other packages to use
	// the convenient `zap.L()` and `zap.S()` functions to get the configured logger.
	zap.ReplaceGlobals(L)
}
