package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewTripMetrics(meter metric.Meter) (*TripMetrics, error) {
	created, err := meter.Int64Counter("tripgo.trips.created",
		metric.WithUnit("{trip}"), metric.WithDescription("Number of created trips."))
	if err != nil {
		return nil, fmt.Errorf("create trips.created counter: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed",
		metric.WithUnit("{trip}"), metric.WithDescription("Number of completed trips."))
	if err != nil {
		return nil, fmt.Errorf("create trips.completed counter: %w", err)
	}
	return &TripMetrics{created: created, completed: completed}, nil
}

func (m *TripMetrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *TripMetrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}

func RegisterActiveTrips(meter metric.Meter, count func(ctx context.Context) (int64, error)) error {
	_, err := meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of trips in progress."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := count(ctx)
			if err != nil {
				return fmt.Errorf("count active trips: %w", err)
			}
			o.Observe(n)
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create trips.active gauge: %w", err)
	}
	return nil
}
