package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

const fieldCoordinates = "coordinates"

func TestNewPositionValidation(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		point      trip.Point
		recordedAt time.Time
		wantField  string
	}{
		{name: "valid", point: pointA, recordedAt: now.Add(-time.Hour)},
		{name: "zero coordinates are valid", point: trip.Point{}, recordedAt: now},
		{name: "minimum bounds", point: trip.Point{Latitude: -90, Longitude: -180}, recordedAt: now},
		{name: "maximum bounds", point: trip.Point{Latitude: 90, Longitude: 180}, recordedAt: now},
		{name: "exactly a minute ahead is valid", point: pointA, recordedAt: now.Add(time.Minute)},
		{name: "latitude too high", point: trip.Point{Latitude: 90.1}, recordedAt: now, wantField: fieldCoordinates},
		{name: "latitude too low", point: trip.Point{Latitude: -90.1}, recordedAt: now, wantField: fieldCoordinates},
		{name: "longitude too high", point: trip.Point{Longitude: 180.1}, recordedAt: now, wantField: fieldCoordinates},
		{name: "longitude too low", point: trip.Point{Longitude: -180.1}, recordedAt: now, wantField: fieldCoordinates},
		{name: "more than a minute ahead", point: pointA, recordedAt: now.Add(time.Minute + time.Nanosecond), wantField: "recorded_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := trip.NewPosition("t1", tc.point, tc.recordedAt, now)

			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, "t1", p.TripID())
				require.Equal(t, tc.point, p.Point())
				return
			}
			var verr *trip.ValidationError
			require.True(t, errors.As(err, &verr))
			require.Equal(t, tc.wantField, verr.Field)
			require.Nil(t, p)
		})
	}
}

func TestNewPositionNormalizesRecordedAt(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	zone := time.FixedZone("UTC+5", 5*60*60)
	recordedAt := time.Date(2026, 11, 14, 15, 0, 0, 123456789, zone)

	p, err := trip.NewPosition("t1", pointA, recordedAt, now)

	require.NoError(t, err)
	require.Equal(t, time.UTC, p.RecordedAt().Location())
	require.Equal(t, time.Date(2026, 11, 14, 10, 0, 0, 123456000, time.UTC), p.RecordedAt())
}

func TestRestorePosition(t *testing.T) {
	recordedAt := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	p := trip.RestorePosition(42, "t1", pointA, recordedAt)

	require.Equal(t, int64(42), p.ID())
	require.Equal(t, "t1", p.TripID())
	require.Equal(t, pointA, p.Point())
	require.Equal(t, recordedAt, p.RecordedAt())
}
