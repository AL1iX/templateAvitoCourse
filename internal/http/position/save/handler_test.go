package save

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
)

const (
	tripID = "11111111-1111-4111-8111-111111111111"

	codeInvalidRequest = "invalid_request"
	fieldCoordinates   = "coordinates"
	fieldRecordedAt    = "recorded_at"
)

var (
	recordedAt = time.Date(2026, 11, 14, 12, 0, 5, 0, time.UTC)
	wantPoint  = trip.Point{Latitude: 59.935, Longitude: 30.335}
)

func newHandler(uc *MockuseCase, c *Mockcanceller) *Handler {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(uc, c, domainerror.New(logger))
}

func body() string {
	return `{"coordinates":{"latitude":59.935,"longitude":30.335},"recorded_at":"2026-11-14T12:00:05Z"}`
}

func serve(uc *MockuseCase, c *Mockcanceller, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID+"/positions", strings.NewReader(raw))
	rec := httptest.NewRecorder()
	newHandler(uc, c).Handle(rec, req, uuid.MustParse(tripID))
	return rec
}

func TestHandle(t *testing.T) {
	saved := trip.RestorePosition(42, tripID, wantPoint, recordedAt)
	errDB := errors.New("password authentication failed")

	tests := []struct {
		name       string
		body       string
		setup      func(uc *MockuseCase)
		wantStatus int
		wantCode   string
		wantCancel bool
	}{
		{
			name: "position is saved",
			body: body(),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ string, point trip.Point, at time.Time) (*trip.Position, error) {
						require.Equal(t, wantPoint, point)
						require.True(t, recordedAt.Equal(at))
						return saved, nil
					},
				)
			},
			wantStatus: http.StatusCreated,
			wantCancel: true,
		},
		{
			name: "trailing whitespace is allowed",
			body: body() + " \n\t ",
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).Return(saved, nil)
			},
			wantStatus: http.StatusCreated,
			wantCancel: true,
		},
		{
			name:       "empty body",
			body:       ``,
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "null body",
			body:       `null`,
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "malformed json",
			body:       `{"coordinates":`,
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "unknown field",
			body:       strings.Replace(body(), `"recorded_at"`, `"foo":1,"recorded_at"`, 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "unknown field inside coordinates",
			body:       strings.Replace(body(), `"longitude"`, `"altitude":1,"longitude"`, 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "second json object after the first is rejected",
			body:       body() + body(),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "garbage after the json object is rejected",
			body:       body() + `}`,
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "recorded_at is not a date",
			body:       strings.Replace(body(), "2026-11-14T12:00:05Z", "yesterday", 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "recorded_at without a time zone",
			body:       strings.Replace(body(), "2026-11-14T12:00:05Z", "2026-11-14T12:00:05", 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "recorded_at is a number",
			body:       strings.Replace(body(), `"2026-11-14T12:00:05Z"`, `1700000000`, 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "latitude is a string",
			body:       strings.Replace(body(), `59.935`, `"59.935"`, 1),
			setup:      func(*MockuseCase) {},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "validation error from the domain",
			body: body(),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).
					Return(nil, &trip.ValidationError{Field: fieldCoordinates, Reason: "out of range"})
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "trip not found",
			body: body(),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).Return(nil, trip.ErrTripNotFound)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name: "trip completed",
			body: body(),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).Return(nil, trip.ErrTripCompleted)
			},
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name: "repository error is a 500 without details",
			body: body(),
			setup: func(uc *MockuseCase) {
				uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).Return(nil, errDB)
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockuseCase(ctrl)
			c := NewMockcanceller(ctrl)
			tc.setup(uc)
			if tc.wantCancel {
				c.EXPECT().Cancel(tripID)
			}

			rec := serve(uc, c, tc.body)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.NotContains(t, rec.Body.String(), "password")
			if tc.wantCode != "" {
				var problem generated.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				require.Equal(t, tc.wantCode, problem.Code)
				require.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
				return
			}

			var got generated.TripPosition
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, int64(42), got.Id)
			require.Equal(t, tripID, got.TripId.String())
			require.Equal(t, generated.Coordinates{Latitude: wantPoint.Latitude, Longitude: wantPoint.Longitude}, got.Coordinates)
			require.True(t, recordedAt.Equal(got.RecordedAt))
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		})
	}
}

func TestHandleRejectsMissingOrNullFields(t *testing.T) {
	paths := [][]string{
		{fieldCoordinates},
		{fieldCoordinates, "latitude"},
		{fieldCoordinates, "longitude"},
		{fieldRecordedAt},
	}

	for _, path := range paths {
		for _, null := range []bool{false, true} {
			name := strings.Join(path, ".") + " missing"
			if null {
				name = strings.Join(path, ".") + " null"
			}

			t.Run(name, func(t *testing.T) {
				var doc map[string]any
				require.NoError(t, json.Unmarshal([]byte(body()), &doc))

				parent := doc
				if len(path) == 2 {
					var ok bool
					parent, ok = doc[path[0]].(map[string]any)
					require.True(t, ok)
				}
				key := path[len(path)-1]
				require.Contains(t, parent, key)
				if null {
					parent[key] = nil
				} else {
					delete(parent, key)
				}
				raw, err := json.Marshal(doc)
				require.NoError(t, err)

				ctrl := gomock.NewController(t)
				uc := NewMockuseCase(ctrl)
				c := NewMockcanceller(ctrl)

				rec := serve(uc, c, string(raw))

				require.Equal(t, http.StatusBadRequest, rec.Code)
				var problem generated.Problem
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
				require.Equal(t, codeInvalidRequest, problem.Code)
			})
		}
	}
}

func TestHandleExplicitZeroCoordinatesAreValid(t *testing.T) {
	const zeroBody = `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-11-14T12:00:05Z"}`

	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	c := NewMockcanceller(ctrl)
	uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, point trip.Point, _ time.Time) (*trip.Position, error) {
			require.Equal(t, trip.Point{}, point)
			return nil, trip.ErrTripCompleted
		},
	)

	rec := serve(uc, c, zeroBody)

	require.Equal(t, http.StatusConflict, rec.Code, "request passed validation and reached the usecase")
}

func TestHandleCancelsQueuedRequestAfterTheUsecaseSucceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMockuseCase(ctrl)
	c := NewMockcanceller(ctrl)
	gomock.InOrder(
		uc.EXPECT().Execute(gomock.Any(), tripID, gomock.Any(), gomock.Any()).
			Return(trip.RestorePosition(42, tripID, wantPoint, recordedAt), nil),
		c.EXPECT().Cancel(tripID),
	)

	rec := serve(uc, c, body())

	require.Equal(t, http.StatusCreated, rec.Code)
}
