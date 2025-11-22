// Package telemetry provides OpenTelemetry tracing and metrics setup.
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

// InitTracerProvider initializes the global trace provider.
// For now, it sets up a no-op or stdout exporter. In production, this would point to a collector.
func InitTracerProvider(ctx context.Context, serviceName string) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// For this refactor, we'll start with a basic setup.
	// In a real scenario, we'd configure an exporter here (e.g. OTLP).
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// sdktrace.WithBatcher(exporter), // Add exporter here
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}
