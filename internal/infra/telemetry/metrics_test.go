package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

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
