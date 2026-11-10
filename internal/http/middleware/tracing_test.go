package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func spanAttr(span sdktrace.ReadOnlySpan, key string) (attribute.Value, bool) {
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func TestTracing(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		wantStatus    int
		wantCode      codes.Code
		wantErrorType string
		wantPanic     bool
	}{
		{"created", statusHandler(http.StatusCreated), http.StatusCreated, codes.Unset, "", false},
		{"client error is not a span error", statusHandler(http.StatusNotFound), http.StatusNotFound, codes.Unset, "", false},
		{"server error", statusHandler(http.StatusInternalServerError), http.StatusInternalServerError, codes.Error, "500", false},
		{panicLabel, panicHandler(), http.StatusInternalServerError, codes.Error, panicLabel, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			h := Tracing(tp, propagation.TraceContext{})(tc.handler)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil).
				WithContext(WithRoute(t.Context(), testRoute))

			serve := func() { h.ServeHTTP(httptest.NewRecorder(), req) }
			if tc.wantPanic {
				require.Panics(t, serve)
			} else {
				serve()
			}

			spans := recorder.Ended()
			require.Len(t, spans, 1)
			span := spans[0]
			require.Equal(t, "POST "+testRoute, span.Name())
			require.Equal(t, tc.wantCode, span.Status().Code)

			status, ok := spanAttr(span, "http.response.status_code")
			require.True(t, ok)
			require.EqualValues(t, tc.wantStatus, status.AsInt64())

			errType, hasErrType := spanAttr(span, "error.type")
			require.Equal(t, tc.wantErrorType != "", hasErrType)
			if hasErrType {
				require.Equal(t, tc.wantErrorType, errType.AsString())
			}
		})
	}
}

func TestTracingContinuesIncomingTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	h := Tracing(tp, propagation.TraceContext{})(statusHandler(http.StatusOK))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil).
		WithContext(WithRoute(t.Context(), testRoute))
	req.Header.Set("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "0af7651916cd43dd8448eb211c80319c", spans[0].SpanContext().TraceID().String())
	require.Equal(t, "b7ad6b7169203331", spans[0].Parent().SpanID().String())
}
