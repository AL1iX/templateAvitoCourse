package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPositionSaved(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	metrics, err := NewTripMetrics(provider.Meter("test"))
	require.NoError(t, err)

	metrics.PositionSaved(t.Context())
	metrics.PositionSaved(t.Context())

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	require.Len(t, rm.ScopeMetrics, 1)
	require.Len(t, rm.ScopeMetrics[0].Metrics, 1)

	m := rm.ScopeMetrics[0].Metrics[0]
	require.Equal(t, "tripgo.positions.saved", m.Name)
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	require.EqualValues(t, 2, sum.DataPoints[0].Value)
}

func TestRegisterActiveTrips(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	err := RegisterActiveTrips(provider.Meter("test"), func(context.Context) (int64, error) {
		return 3, nil
	})
	require.NoError(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))
	require.Len(t, rm.ScopeMetrics, 1)
	require.Len(t, rm.ScopeMetrics[0].Metrics, 1)

	m := rm.ScopeMetrics[0].Metrics[0]
	require.Equal(t, "tripgo.trips.active", m.Name)

	gauge, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, gauge.DataPoints, 1)
	require.EqualValues(t, 3, gauge.DataPoints[0].Value)
}
