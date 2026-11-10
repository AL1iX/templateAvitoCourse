package middleware

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func Metrics(meter metric.Meter) (func(http.Handler) http.Handler, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("create http duration histogram: %w", err)
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of in-flight HTTP server requests."),
	)
	if err != nil {
		return nil, fmt.Errorf("create http active requests counter: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			labels := []attribute.KeyValue{
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", RouteFromContext(ctx)),
			}
			active.Add(ctx, 1, metric.WithAttributes(labels...))

			start := time.Now()
			sw := wrap(w)
			panicked := true

			defer func() {
				status := sw.Status()
				if panicked {
					status = http.StatusInternalServerError
				}

				active.Add(ctx, -1, metric.WithAttributes(labels...))
				withStatus := append(slices.Clone(labels), attribute.Int("http.response.status_code", status))
				duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(withStatus...))
			}()

			next.ServeHTTP(sw, r)
			panicked = false
		})
	}, nil
}
