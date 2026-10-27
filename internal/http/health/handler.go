package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

type Handler struct {
	db      pinger
	timeout time.Duration
	logger  *slog.Logger
}

func New(db pinger, timeout time.Duration, logger *slog.Logger) *Handler {
	return &Handler{db: db, timeout: timeout, logger: logger}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, generated.Ok)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.logger.WarnContext(r.Context(), "readiness check failed", "error", err)
		write(w, http.StatusServiceUnavailable, generated.Unavailable)
		return
	}
	write(w, http.StatusOK, generated.Ok)
}

func write(w http.ResponseWriter, status int, value generated.HealthResponseStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"status":"` + string(value) + `"}`))
}
