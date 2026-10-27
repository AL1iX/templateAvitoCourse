package telemetry

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTraceHandler(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(traceHandler{Handler: slog.NewJSONHandler(&logs, nil)}).With("service", "trip-service")

	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "op")
	defer span.End()

	logger.InfoContext(ctx, "inside span")
	logger.InfoContext(t.Context(), "outside span")

	lines := bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)

	var inside, outside map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &inside))
	require.NoError(t, json.Unmarshal(lines[1], &outside))

	require.Equal(t, span.SpanContext().TraceID().String(), inside["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), inside["span_id"])
	require.Equal(t, "trip-service", inside["service"])
	require.NotContains(t, outside, "trace_id")
	require.NotContains(t, outside, "span_id")
	require.Equal(t, "trip-service", outside["service"])
}
