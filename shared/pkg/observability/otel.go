// Package observability provides OpenTelemetry bootstrap for traces and metrics.
package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config holds OpenTelemetry bootstrap configuration.
type Config struct {
	ServiceName    string
	OTLPEndpoint   string // gRPC endpoint for OTLP collector (e.g. "localhost:4317")
	TraceEnabled   bool
	MetricsEnabled bool
}

// Shutdown is a function that flushes and shuts down telemetry providers.
type Shutdown func(ctx context.Context) error

// Setup initializes OpenTelemetry trace and metric providers.
// Call the returned Shutdown function during graceful shutdown.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	var shutdowns []func(context.Context) error

	if cfg.TraceEnabled && cfg.OTLPEndpoint != "" {
		traceExp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating trace exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExp),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tp)
		shutdowns = append(shutdowns, tp.Shutdown)
	}

	if cfg.MetricsEnabled {
		promExp, err := prometheus.New()
		if err != nil {
			return nil, fmt.Errorf("creating prometheus exporter: %w", err)
		}
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(promExp),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, mp.Shutdown)
	}

	shutdown := func(ctx context.Context) error {
		var firstErr error
		for _, fn := range shutdowns {
			if err := fn(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	return shutdown, nil
}

// Meter returns a named meter from the global meter provider.
func Meter(name string) metric.Meter {
	return otel.GetMeterProvider().Meter(name)
}

// ShutdownTimeout is the default timeout for flushing telemetry on shutdown.
const ShutdownTimeout = 5 * time.Second
