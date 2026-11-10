package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %s not found", name)
	return metricdata.Metrics{}
}

func TestMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mw, err := Metrics(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil).
		WithContext(WithRoute(t.Context(), testRoute))
	mw(statusHandler(http.StatusCreated)).ServeHTTP(httptest.NewRecorder(), req)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	duration := findMetric(t, rm, "http.server.request.duration")
	require.Equal(t, "s", duration.Unit)
	hist, ok := duration.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	point := hist.DataPoints[0]
	require.EqualValues(t, 1, point.Count)

	route, ok := point.Attributes.Value("http.route")
	require.True(t, ok)
	require.Equal(t, testRoute, route.AsString())
	status, ok := point.Attributes.Value("http.response.status_code")
	require.True(t, ok)
	require.EqualValues(t, http.StatusCreated, status.AsInt64())
	method, ok := point.Attributes.Value("http.request.method")
	require.True(t, ok)
	require.Equal(t, http.MethodPost, method.AsString())

	active := findMetric(t, rm, "http.server.active_requests")
	sum, ok := active.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	require.EqualValues(t, 0, sum.DataPoints[0].Value, "gauge returns to zero after the request")
}
