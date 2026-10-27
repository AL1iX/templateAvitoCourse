package domainerror

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"validation", &trip.ValidationError{Field: "price", Reason: "must not be negative"}, http.StatusBadRequest, "invalid_request"},
		{"wrapped validation", fmt.Errorf("create trip: %w", &trip.ValidationError{Field: "body", Reason: "bad"}), http.StatusBadRequest, "invalid_request"},
		{"trip not found", fmt.Errorf("get trip: %w", trip.ErrTripNotFound), http.StatusNotFound, "trip_not_found"},
		{"trip completed", fmt.Errorf("finish trip: %w", trip.ErrTripCompleted), http.StatusConflict, "trip_completed"},
		{"driver busy", fmt.Errorf("create trip: %w", trip.ErrDriverBusy), http.StatusConflict, "driver_busy"},
		{"idempotency conflict", trip.ErrIdempotencyConflict, http.StatusConflict, "idempotency_conflict"},
		{"unknown error", errors.New("something broke"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := Problem(tc.err)

			require.Equal(t, tc.wantStatus, status)
			require.EqualValues(t, tc.wantStatus, body.Status)
			require.Equal(t, tc.wantCode, body.Code)
		})
	}
}

func TestWriteHidesInternalDetails(t *testing.T) {
	var logs bytes.Buffer
	mapper := New(slog.New(slog.NewJSONHandler(&logs, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/42", nil)
	rec := httptest.NewRecorder()

	mapper.Write(rec, req, errors.New("password authentication failed for user postgres"))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
	require.NotContains(t, rec.Body.String(), "password")
	require.Contains(t, rec.Body.String(), `"instance":"/api/v1/trips/42"`)
	require.Contains(t, logs.String(), "password authentication failed")
}
