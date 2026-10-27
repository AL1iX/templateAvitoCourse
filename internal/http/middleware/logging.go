package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := wrap(w)
			panicked := true

			defer func() {
				status := sw.Status()
				if panicked {
					status = http.StatusInternalServerError
				}

				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				}

				logger.Log(r.Context(), level, "request handled",
					"method", r.Method,
					"route", RouteFromContext(r.Context()),
					"status", status,
					"duration_ms", float64(time.Since(start).Microseconds())/1000,
				)
			}()

			next.ServeHTTP(sw, r)
			panicked = false
		})
	}
}
