package create

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

const (
	tripID   = "11111111-1111-4111-8111-111111111111"
	userID   = "22222222-2222-4222-8222-222222222222"
	driverID = "33333333-3333-4333-8333-333333333333"

	codeInvalidRequest = "invalid_request"
)

func newHandler(uc *MockuseCase) *Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(uc, domainerror.New(logger))
}

func body(user string) string {
	return `{"user_id":"` + user + `","driver_id":"` + driverID +
		`","start_point":{"latitude":56.8,"longitude":60.6},"end_point":{"latitude":56.9,"longitude":60.7},"price":500}`
}

func TestHandle(t *testing.T) {
	created := trip.Restore(tripID, userID, driverID,
		trip.Point{Latitude: 56.8, Longitude: 60.6}, trip.Point{Latitude: 56.9, Longitude: 60.7},
		500, trip.StatusActive, time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC), nil)
	errDB := errors.New("password authentication failed")

	tests := []struct {
		name         string
		body         string
		setup        func(uc *MockuseCase)
		wantStatus   int
		wantCode     string
		wantLocation bool
	}{
		{
			name: "trip is created",
			body: body(userID),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{Trip: created}, nil)
			},
			wantStatus:   http.StatusCreated,
			wantLocation: true,
		},
		{
			name: "replayed request",
			body: body(userID),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(model.CreateTripResult{Trip: created, Replayed: true}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "malformed json never reaches the usecase",
			body:       `{"user_id":`,
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "unknown field",
			body:       strings.Replace(body(userID), `"price":500`, `"price":500,"foo":1`, 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "nil user id",
			body:       body(uuid.Nil.String()),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "validation error from the domain",
			body: body(userID),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(model.CreateTripResult{}, &trip.ValidationError{Field: "price", Reason: "must not be negative"})
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "driver is busy",
			body: body(userID),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(model.CreateTripResult{}, trip.ErrDriverBusy)
			},
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name: "repository error is a 500 without details",
			body: body(userID),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
					Return(model.CreateTripResult{}, errDB)
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uc := NewMockuseCase(gomock.NewController(t))
			tc.setup(uc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			newHandler(uc).Handle(rec, req, generated.CreateTripParams{})

			require.Equal(t, tc.wantStatus, rec.Code)
			require.NotContains(t, rec.Body.String(), "password")
			if tc.wantCode != "" {
				var problem generated.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				require.Equal(t, tc.wantCode, problem.Code)
				require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
				return
			}

			var got generated.Trip
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, tripID, got.Id.String())
			require.Equal(t, tc.wantLocation, rec.Header().Get("Location") != "")
		})
	}
}

func TestHandleIdempotencyKey(t *testing.T) {
	key := uuid.New()
	uc := NewMockuseCase(gomock.NewController(t))
	uc.EXPECT().Execute(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
			require.Equal(t, key.String(), in.IdempotencyKey)
			require.NotEmpty(t, in.RequestHash)
			return model.CreateTripResult{}, trip.ErrIdempotencyConflict
		})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body(userID)))
	rec := httptest.NewRecorder()

	newHandler(uc).Handle(rec, req, generated.CreateTripParams{IdempotencyKey: &key})

	require.Equal(t, http.StatusConflict, rec.Code)
}
