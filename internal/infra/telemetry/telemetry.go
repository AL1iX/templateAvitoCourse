package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	defaultServiceName = "trip-service"
	serviceVersion     = "dev"
	metricInterval     = 10 * time.Second
)

type Telemetry struct {
	Logger         *slog.Logger
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Propagator     propagation.TextMapPropagator
}

func New(ctx context.Context, level slog.Level) (*Telemetry, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", defaultServiceName),
			attribute.String("service.namespace", "tripgo"),
			attribute.String("service.version", serviceVersion),
			attribute.String("deployment.environment", "local"),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	service := defaultServiceName
	if v, ok := res.Set().Value("service.name"); ok {
		service = v.AsString()
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(metricInterval))),
		sdkmetric.WithResource(res),
	)

	return &Telemetry{
		Logger:         newLogger(level, service),
		TracerProvider: tp,
		MeterProvider:  mp,
		Propagator:     propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(t.MeterProvider.Shutdown(ctx), t.TracerProvider.Shutdown(ctx))
}
