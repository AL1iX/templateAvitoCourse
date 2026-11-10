package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

var (
	pointA    = trip.Point{Latitude: 59.9, Longitude: 30.3}
	pointB    = trip.Point{Latitude: 59.9, Longitude: 30.4}
	startedAt = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
)

func newTrip(t *testing.T) *trip.Trip {
	t.Helper()
	tr, err := trip.New("t1", "u1", "d1", pointA, pointB, 100, startedAt)
	require.NoError(t, err)
	return tr
}

func TestTripFinish(t *testing.T) {
	first := startedAt.Add(time.Hour)
	second := startedAt.Add(2 * time.Hour)

	tests := []struct {
		name            string
		alreadyFinished bool
		wantErr         error
		wantFinishedAt  time.Time
	}{
		{name: "active trip is completed", wantFinishedAt: second},
		{name: "completed trip is not finished again", alreadyFinished: true, wantErr: trip.ErrTripCompleted, wantFinishedAt: first},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTrip(t)
			if tc.alreadyFinished {
				require.NoError(t, tr.Finish(first))
			}

			err := tr.Finish(second)

			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, trip.StatusCompleted, tr.Status())
			require.NotNil(t, tr.FinishedAt())
			require.Equal(t, tc.wantFinishedAt, *tr.FinishedAt())
		})
	}
}

func TestNewValidation(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		driverID  string
		start     trip.Point
		end       trip.Point
		price     int64
		wantField string
	}{
		{name: "valid", userID: "u1", driverID: "d1", start: pointA, end: pointB, price: 100},
		{name: "minimum bounds and zero price", userID: "u1", driverID: "d1", start: trip.Point{Latitude: -90, Longitude: -180}, end: pointB, price: 0},
		{name: "maximum bounds", userID: "u1", driverID: "d1", start: pointA, end: trip.Point{Latitude: 90, Longitude: 180}, price: 100},
		{name: "empty user", userID: "", driverID: "d1", start: pointA, end: pointB, price: 100, wantField: "user_id, driver_id"},
		{name: "empty driver", userID: "u1", driverID: "", start: pointA, end: pointB, price: 100, wantField: "user_id, driver_id"},
		{name: "start latitude out of range", userID: "u1", driverID: "d1", start: trip.Point{Latitude: 91}, end: pointB, price: 100, wantField: "start_point"},
		{name: "end longitude out of range", userID: "u1", driverID: "d1", start: pointA, end: trip.Point{Longitude: -181}, price: 100, wantField: "end_point"},
		{name: "negative price", userID: "u1", driverID: "d1", start: pointA, end: pointB, price: -1, wantField: "price"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := trip.New("t1", tc.userID, tc.driverID, tc.start, tc.end, tc.price, startedAt)

			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, trip.StatusActive, tr.Status())
				return
			}
			var verr *trip.ValidationError
			require.True(t, errors.As(err, &verr))
			require.Equal(t, tc.wantField, verr.Field)
			require.Nil(t, tr)
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	err := &trip.ValidationError{Field: "start_point", Reason: "coordinates out of range"}

	require.EqualError(t, err, "start_point: coordinates out of range")
}
