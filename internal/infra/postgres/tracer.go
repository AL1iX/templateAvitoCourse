package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var tableRe = regexp.MustCompile(`(?i)\b(?:from|into|update)\s+([a-z_][a-z0-9_]*)`)

type queryKey struct{}

type queryInfo struct {
	start time.Time
	attrs []attribute.KeyValue
}

type queryTracer struct {
	spans    pgx.QueryTracer
	duration metric.Float64Histogram
}

func NewQueryTracer(tp trace.TracerProvider, meter metric.Meter) (pgx.QueryTracer, error) {
	duration, err := meter.Float64Histogram("db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5),
	)
	if err != nil {
		return nil, fmt.Errorf("create db duration histogram: %w", err)
	}
	return &queryTracer{
		spans:    otelpgx.NewTracer(otelpgx.WithTracerProvider(tp)),
		duration: duration,
	}, nil
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.spans.TraceQueryStart(ctx, conn, data)

	attrs := queryAttributes(data.SQL)
	trace.SpanFromContext(ctx).SetAttributes(attrs...)

	return context.WithValue(ctx, queryKey{}, queryInfo{start: time.Now(), attrs: attrs})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if info, ok := ctx.Value(queryKey{}).(queryInfo); ok {
		attrs := info.attrs
		if data.Err != nil {
			errType := attribute.String("error.type", errorType(data.Err))
			trace.SpanFromContext(ctx).SetAttributes(errType)
			attrs = append(slices.Clone(attrs), errType)
		}
		t.duration.Record(ctx, time.Since(info.start).Seconds(), metric.WithAttributes(attrs...))
	}

	t.spans.TraceQueryEnd(ctx, conn, data)
}

func queryAttributes(sql string) []attribute.KeyValue {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return nil
	}

	attrs := []attribute.KeyValue{attribute.String("db.operation.name", strings.ToUpper(fields[0]))}
	if m := tableRe.FindStringSubmatch(sql); m != nil {
		attrs = append(attrs, attribute.String("db.collection.name", m[1]))
	}
	return attrs
}

func errorType(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return "_OTHER"
}
