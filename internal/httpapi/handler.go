package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AL1iX/templateAvitoCourse/api"
	"github.com/AL1iX/templateAvitoCourse/internal/config"
	"github.com/AL1iX/templateAvitoCourse/internal/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/txmanager"
)

type Handler struct {
	api.Unimplemented

	pool   *pgxpool.Pool
	logger *slog.Logger
	cfg    config.Config

	tx   *txmanager.TxManager
	trip *trip.Repository
}

func NewHandler(pool *pgxpool.Pool, logger *slog.Logger, cfg config.Config, tx *txmanager.TxManager, tripRepo *trip.Repository) *Handler {
	return &Handler{pool: pool, logger: logger, cfg: cfg, tx: tx, trip: tripRepo}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeHealth(w, http.StatusOK, api.Ok)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.DatabaseConnectTimeout)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		h.logger.Warn("readiness check failed", "error", err)
		writeHealth(w, http.StatusServiceUnavailable, api.Unavailable)
		return
	}
	writeHealth(w, http.StatusOK, api.Ok)
}

func writeHealth(w http.ResponseWriter, status int, value api.HealthResponseStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"status":"` + string(value) + `"}`))
}

const (
	typeInvalidRequest = "https://tripgo.example/problems/invalid-request"
	typeTripNotFound   = "https://tripgo.example/problems/trip-not-found"
	typeTripCompleted  = "https://tripgo.example/problems/trip-completed"
	typeDriverBusy     = "https://tripgo.example/problems/driver-busy"
	typeInternalError  = "https://tripgo.example/problems/internal-error"
)

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var body api.TripData
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, typeInvalidRequest, "invalid_request", "Invalid request", "request body: "+err.Error())
		return
	}
	if body.UserId == uuid.Nil || body.DriverId == uuid.Nil {
		writeProblem(w, r, http.StatusBadRequest, typeInvalidRequest, "invalid_request", "Invalid request", "user_id and driver_id are required")
		return
	}
	if !validCoordinates(body.StartPoint) || !validCoordinates(body.EndPoint) {
		writeProblem(w, r, http.StatusBadRequest, typeInvalidRequest, "invalid_request", "Invalid request", "coordinates out of range")
		return
	}
	if body.Price < 0 {
		writeProblem(w, r, http.StatusBadRequest, typeInvalidRequest, "invalid_request", "Invalid request", "price must not be negative")
		return
	}

	t := trip.Trip{
		ID:             uuid.New().String(),
		UserID:         body.UserId.String(),
		DriverID:       body.DriverId.String(),
		StartLatitude:  body.StartPoint.Latitude,
		StartLongitude: body.StartPoint.Longitude,
		EndLatitude:    body.EndPoint.Latitude,
		EndLongitude:   body.EndPoint.Longitude,
		Price:          body.Price,
		Status:         trip.StatusActive,
		StartedAt:      time.Now().UTC(),
	}

	err := h.tx.Do(r.Context(), func(ctx context.Context) error {
		if err := h.trip.Create(ctx, t); err != nil {
			return err
		}
		return h.trip.AddStatusHistory(ctx, t.ID, nil, trip.StatusActive, "trip created")
	})
	switch {
	case errors.Is(err, trip.ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict, typeDriverBusy, "driver_busy", "Driver busy", "driver already has an active trip")
		return
	case err != nil:
		h.logger.Error("create trip failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, typeInternalError, "internal_error", "Internal error", "")
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+t.ID)
	writeJSON(w, http.StatusCreated, toAPITrip(t))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	t, err := h.trip.GetByID(r.Context(), tripId.String())
	switch {
	case errors.Is(err, trip.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, typeTripNotFound, "trip_not_found", "Trip not found", "")
		return
	case err != nil:
		h.logger.Error("get trip failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, typeInternalError, "internal_error", "Internal error", "")
		return
	}
	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	id := tripId.String()
	now := time.Now().UTC()

	err := h.tx.Do(r.Context(), func(ctx context.Context) error {
		if err := h.trip.Finish(ctx, id, now); err != nil {
			return err
		}
		from := trip.StatusActive
		return h.trip.AddStatusHistory(ctx, id, &from, trip.StatusCompleted, "trip finished")
	})
	switch {
	case errors.Is(err, trip.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, typeTripNotFound, "trip_not_found", "Trip not found", "")
		return
	case errors.Is(err, trip.ErrCompleted):
		writeProblem(w, r, http.StatusConflict, typeTripCompleted, "trip_completed", "Trip already completed", "")
		return
	case err != nil:
		h.logger.Error("finish trip failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, typeInternalError, "internal_error", "Internal error", "")
		return
	}

	t, err := h.trip.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("reload finished trip failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, typeInternalError, "internal_error", "Internal error", "")
		return
	}
	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func validCoordinates(c api.Coordinates) bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

func toAPITrip(t trip.Trip) api.Trip {
	return api.Trip{
		Id:         uuid.MustParse(t.ID),
		UserId:     uuid.MustParse(t.UserID),
		DriverId:   uuid.MustParse(t.DriverID),
		StartPoint: api.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude},
		EndPoint:   api.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, problemType, code, title, detail string) {
	instance := r.URL.Path
	p := api.Problem{
		Type:     problemType,
		Title:    title,
		Status:   int32(status),
		Code:     code,
		Instance: &instance,
	}
	if detail != "" {
		p.Detail = &detail
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
