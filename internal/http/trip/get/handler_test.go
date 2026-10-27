package get

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/api"
	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
)

const (
	tripID   = "11111111-1111-4111-8111-111111111111"
	userID   = "22222222-2222-4222-8222-222222222222"
	driverID = "33333333-3333-4333-8333-333333333333"
)

func TestHandle(t *testing.T) {
	found := trip.Restore(tripID, userID, driverID,
		trip.Point{Latitude: 56.8, Longitude: 60.6}, trip.Point{Latitude: 56.9, Longitude: 60.7},
		500, trip.StatusActive, time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC), nil)

	tests := []struct {
		name       string
		result     *trip.Trip
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "trip found", result: found, wantStatus: http.StatusOK},
		{
			name:       "trip not found",
			err:        fmt.Errorf("get trip: %w", trip.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "repository error",
			err:        errors.New("password authentication failed"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockuseCase(gomock.NewController(t))
			uc.EXPECT().Execute(gomock.Any(), tripID).Return(tc.result, tc.err)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
			h := New(uc, domainerror.New(logger))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID, nil)
			rec := httptest.NewRecorder()

			h.Handle(rec, req, uuid.MustParse(tripID))

			require.Equal(t, tc.wantStatus, rec.Code)
			require.NotContains(t, rec.Body.String(), "password")
			if tc.wantCode != "" {
				var problem api.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				require.Equal(t, tc.wantCode, problem.Code)
				return
			}

			var got api.Trip
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tripID, got.Id.String())
			require.Equal(t, api.TripStatus("active"), got.Status)
		})
	}
}
