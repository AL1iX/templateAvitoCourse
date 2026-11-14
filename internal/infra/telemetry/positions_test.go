package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func collectMetrics(t *testing.T, reader sdkmetric.Reader) map[string]metricdata.Metrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	byName := make(map[string]metricdata.Metrics)
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			byName[m.Name] = m
		}
	}
	return byName
}

func countsByResult(t *testing.T, m metricdata.Metrics) map[string]int64 {
	t.Helper()

	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	counts := make(map[string]int64)
	for _, dp := range sum.DataPoints {
		result, found := dp.Attributes.Value("result")
		require.True(t, found, "every data point has the result attribute")
		counts[result.AsString()] = dp.Value
	}
	return counts
}

func TestPositionMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	metrics, err := NewPositionMetrics(provider.Meter("test"))
	require.NoError(t, err)

	metrics.StaleCheck(t.Context(), "queued")
	metrics.StaleCheck(t.Context(), "queued")
	metrics.StaleCheck(t.Context(), "duplicate")
	metrics.StaleCheck(t.Context(), "dropped")
	metrics.PositionRequest(t.Context(), "success")
	metrics.PositionRequest(t.Context(), "error")
	metrics.PositionRequest(t.Context(), "error")
	metrics.PositionRequest(t.Context(), "dropped")
	metrics.PositionRequestDuration(t.Context(), "success", 1500*time.Millisecond)

	collected := collectMetrics(t, reader)

	require.Equal(t, map[string]int64{"queued": 2, "duplicate": 1, "dropped": 1},
		countsByResult(t, collected["tripgo.stale_checks"]))
	require.Equal(t, map[string]int64{"success": 1, "error": 2, "dropped": 1},
		countsByResult(t, collected["tripgo.position_requests"]))

	histogram, ok := collected["tripgo.position_requests.duration"].Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, histogram.DataPoints, 1)
	point := histogram.DataPoints[0]
	result, found := point.Attributes.Value("result")
	require.True(t, found)
	require.Equal(t, "success", result.AsString())
	require.EqualValues(t, 1, point.Count)
	require.InDelta(t, 1.5, point.Sum, 1e-9, "the duration is recorded in seconds")
	require.Equal(t, "s", collected["tripgo.position_requests.duration"].Unit)
}

func TestPositionRequestDurationBucketsAreInSeconds(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	metrics, err := NewPositionMetrics(provider.Meter("test"))
	require.NoError(t, err)

	metrics.PositionRequestDuration(t.Context(), "success", 150*time.Millisecond)

	histogram, ok := collectMetrics(t, reader)["tripgo.position_requests.duration"].Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	point := histogram.DataPoints[0]
	require.LessOrEqual(t, point.Bounds[0], 0.01, "milliseconds-scale buckets would put every call into one bucket")
	var upper float64
	for i, count := range point.BucketCounts {
		if count > 0 {
			upper = point.Bounds[i]
		}
	}
	require.InDelta(t, 0.25, upper, 1e-9, "150 ms lands in the bucket up to 250 ms")
}

func TestRegisterPositionQueue(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	err := RegisterPositionQueue(provider.Meter("test"), func() int { return 7 }, func() int { return 3 })
	require.NoError(t, err)

	collected := collectMetrics(t, reader)

	for name, want := range map[string]int64{
		"tripgo.position_requests.queue.size":     7,
		"tripgo.position_requests.workers.active": 3,
	} {
		gauge, ok := collected[name].Data.(metricdata.Gauge[int64])
		require.True(t, ok, name)
		require.Len(t, gauge.DataPoints, 1, name)
		require.Equal(t, want, gauge.DataPoints[0].Value, name)
	}
}
