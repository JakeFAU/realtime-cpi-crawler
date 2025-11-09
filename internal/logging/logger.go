// Package logging provides zap logger helpers.
package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New builds a zap.Logger configured for development or production.
func New(development bool) (*zap.Logger, error) {
	if development {
		cfg := zap.NewDevelopmentConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		logger, err := cfg.Build()
		if err != nil {
			return nil, fmt.Errorf("build dev logger: %w", err)
		}
		return logger, nil
	}
	cfg := zap.NewProductionConfig()
	cfg.DisableStacktrace = false
	cfg.EncoderConfig.TimeKey = "ts"
	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build prod logger: %w", err)
	}
	return logger, nil
}
