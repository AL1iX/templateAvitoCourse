package container

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/AL1iX/templateAvitoCourse/internal/http/middleware"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/telemetry"
)

type metrics struct {
	trips       *telemetry.TripMetrics
	httpMiddle  func(http.Handler) http.Handler
	queryTracer pgx.QueryTracer
}

func newMetrics(tel *telemetry.Telemetry) (metrics, error) {
	meter := tel.MeterProvider.Meter(instrumentationName)

	trips, err := telemetry.NewTripMetrics(meter)
	if err != nil {
		return metrics{}, fmt.Errorf("create trip metrics: %w", err)
	}

	httpMiddle, err := middleware.Metrics(meter)
	if err != nil {
		return metrics{}, fmt.Errorf("create http metrics: %w", err)
	}

	queryTracer, err := postgres.NewQueryTracer(tel.TracerProvider, meter)
	if err != nil {
		return metrics{}, fmt.Errorf("create query tracer: %w", err)
	}

	return metrics{trips: trips, httpMiddle: httpMiddle, queryTracer: queryTracer}, nil
}
