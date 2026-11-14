package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var positionDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var staleScanRowsBuckets = []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

type PositionMetrics struct {
	staleChecks metric.Int64Counter
	scanRows    metric.Int64Histogram
	requests    metric.Int64Counter
	duration    metric.Float64Histogram
}

func NewPositionMetrics(meter metric.Meter) (*PositionMetrics, error) {
	staleChecks, err := meter.Int64Counter("tripgo.stale_checks",
		metric.WithUnit("{trip}"), metric.WithDescription("What the scanner did with a stale trip."))
	if err != nil {
		return nil, fmt.Errorf("create stale_checks counter: %w", err)
	}
	scanRows, err := meter.Int64Histogram("tripgo.stale_scan.rows",
		metric.WithUnit("{row}"), metric.WithDescription("Rows the scanner read from the database during one tick."),
		metric.WithExplicitBucketBoundaries(staleScanRowsBuckets...))
	if err != nil {
		return nil, fmt.Errorf("create stale_scan.rows histogram: %w", err)
	}
	requests, err := meter.Int64Counter("tripgo.position_requests",
		metric.WithUnit("{request}"), metric.WithDescription("Outcome of a position request."))
	if err != nil {
		return nil, fmt.Errorf("create position_requests counter: %w", err)
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration",
		metric.WithUnit("s"), metric.WithDescription("Duration of the Push Service call."),
		metric.WithExplicitBucketBoundaries(positionDurationBuckets...))
	if err != nil {
		return nil, fmt.Errorf("create position_requests.duration histogram: %w", err)
	}
	return &PositionMetrics{staleChecks: staleChecks, scanRows: scanRows, requests: requests, duration: duration}, nil
}

func (m *PositionMetrics) StaleScanRows(ctx context.Context, rows int) {
	m.scanRows.Record(ctx, int64(rows))
}

func (m *PositionMetrics) StaleCheck(ctx context.Context, result string) {
	m.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) PositionRequest(ctx context.Context, result string) {
	m.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *PositionMetrics) PositionRequestDuration(ctx context.Context, result string, d time.Duration) {
	m.duration.Record(ctx, d.Seconds(), metric.WithAttributes(attribute.String("result", result)))
}

func RegisterPositionQueue(meter metric.Meter, queueSize, activeWorkers func() int) error {
	_, err := meter.Int64ObservableGauge("tripgo.position_requests.queue.size",
		metric.WithUnit("{task}"),
		metric.WithDescription("Tasks waiting in the position request queue."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(queueSize()))
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create position_requests.queue.size gauge: %w", err)
	}

	_, err = meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"),
		metric.WithDescription("Workers busy with a position request."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(activeWorkers()))
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create position_requests.workers.active gauge: %w", err)
	}
	return nil
}
