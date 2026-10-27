package middleware

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/AL1iX/templateAvitoCourse/internal/http/middleware"

func Tracing(tp trace.TracerProvider, prop propagation.TextMapPropagator) func(http.Handler) http.Handler {
	tracer := tp.Tracer(tracerName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := RouteFromContext(r.Context())

			ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method+" "+route,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("http.route", route),
				),
			)
			defer span.End()

			sw := wrap(w)
			panicked := true
			defer func() {
				status := sw.Status()
				errType := strconv.Itoa(status)
				if panicked {
					status = http.StatusInternalServerError
					errType = "panic"
				}

				span.SetAttributes(attribute.Int("http.response.status_code", status))
				if status >= http.StatusInternalServerError {
					span.SetStatus(codes.Error, http.StatusText(status))
					span.SetAttributes(attribute.String("error.type", errType))
				}
			}()

			next.ServeHTTP(sw, r.WithContext(ctx))
			panicked = false
		})
	}
}
