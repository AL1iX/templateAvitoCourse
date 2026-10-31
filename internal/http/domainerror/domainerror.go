package domainerror

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

const (
	typeInvalidRequest      = "https://tripgo.example/problems/invalid-request"
	typeTripNotFound        = "https://tripgo.example/problems/trip-not-found"
	typeTripCompleted       = "https://tripgo.example/problems/trip-completed"
	typeDriverBusy          = "https://tripgo.example/problems/driver-busy"
	typeIdempotencyConflict = "https://tripgo.example/problems/idempotency-conflict"
	typeInternalError       = "https://tripgo.example/problems/internal-error"
)

type Mapper struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *Mapper {
	return &Mapper{logger: logger}
}

func Problem(err error) (int, generated.Problem) {
	var verr *trip.ValidationError

	switch {
	case errors.As(err, &verr):
		return build(http.StatusBadRequest, typeInvalidRequest, "invalid_request", "Invalid request", verr.Error())
	case errors.Is(err, trip.ErrTripNotFound):
		return build(http.StatusNotFound, typeTripNotFound, "trip_not_found", "Trip not found", "")
	case errors.Is(err, trip.ErrTripCompleted):
		return build(http.StatusConflict, typeTripCompleted, "trip_completed", "Trip already completed", "")
	case errors.Is(err, trip.ErrDriverBusy):
		return build(http.StatusConflict, typeDriverBusy, "driver_busy", "Driver busy", "driver already has an active trip")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		return build(http.StatusConflict, typeIdempotencyConflict, "idempotency_conflict", "Idempotency conflict",
			"Idempotency-Key was already used with a different request body")
	default:
		return build(http.StatusInternalServerError, typeInternalError, "internal_error", "Internal error", "")
	}
}

func build(status int32, problemType, code, title, detail string) (int, generated.Problem) {
	p := generated.Problem{
		Type:   problemType,
		Title:  title,
		Status: status,
		Code:   code,
	}
	if detail != "" {
		p.Detail = &detail
	}
	return int(status), p
}

func (m *Mapper) Write(w http.ResponseWriter, r *http.Request, err error) {
	status, body := Problem(err)
	if status >= http.StatusInternalServerError {
		m.logger.ErrorContext(r.Context(), "request failed", "error", err)
	}

	instance := r.URL.Path
	body.Instance = &instance

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
