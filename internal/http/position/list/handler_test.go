package list

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
)

const tripID = "11111111-1111-4111-8111-111111111111"

func newHandler(uc *MockuseCase) *Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(uc, domainerror.New(logger))
}

func serve(uc *MockuseCase) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID+"/positions", nil)
	rec := httptest.NewRecorder()
	newHandler(uc).Handle(rec, req, uuid.MustParse(tripID))
	return rec
}

func TestHandleReturnsRouteInTheOrderOfTheUsecase(t *testing.T) {
	first := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	positions := []*trip.Position{
		trip.RestorePosition(7, tripID, trip.Point{Latitude: 1, Longitude: 2}, first),
		trip.RestorePosition(5, tripID, trip.Point{Latitude: 3, Longitude: 4}, first.Add(time.Second)),
	}
	uc := NewMockuseCase(gomock.NewController(t))
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(positions, nil)

	rec := serve(uc)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var got generated.TripPositionsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Positions, 2)
	require.Equal(t, int64(7), got.Positions[0].Id)
	require.Equal(t, int64(5), got.Positions[1].Id)
	require.Equal(t, tripID, got.Positions[0].TripId.String())
	require.Equal(t, generated.Coordinates{Latitude: 3, Longitude: 4}, got.Positions[1].Coordinates)
	require.True(t, first.Equal(got.Positions[0].RecordedAt))
}

func TestHandleEmptyRouteIsAnEmptyArray(t *testing.T) {
	uc := NewMockuseCase(gomock.NewController(t))
	uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, nil)

	rec := serve(uc)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"positions":[]}`, rec.Body.String())
}

func TestHandleErrors(t *testing.T) {
	errDB := errors.New("password authentication failed")

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "trip not found", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "repository error is a 500 without details", err: errDB, wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockuseCase(gomock.NewController(t))
			uc.EXPECT().Execute(gomock.Any(), tripID).Return(nil, tc.err)

			rec := serve(uc)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.NotContains(t, rec.Body.String(), "password")
			require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			var problem generated.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			require.Equal(t, tc.wantCode, problem.Code)
		})
	}
}
