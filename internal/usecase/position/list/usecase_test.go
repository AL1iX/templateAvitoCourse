package list

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

const tripID = "t1"

func TestExecute(t *testing.T) {
	startedAt := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	existing := trip.Restore(tripID, "u1", "d1", trip.Point{}, trip.Point{}, 100, trip.StatusActive, startedAt, nil)
	positions := []*trip.Position{
		trip.RestorePosition(1, tripID, trip.Point{Latitude: 1, Longitude: 2}, startedAt),
		trip.RestorePosition(2, tripID, trip.Point{Latitude: 3, Longitude: 4}, startedAt.Add(time.Second)),
	}
	errDB := errors.New("db down")

	tests := []struct {
		name          string
		tripErr       error
		positions     []*trip.Position
		positionsErr  error
		listIsCalled  bool
		wantErr       error
		wantPositions []*trip.Position
	}{
		{name: "route is returned", positions: positions, listIsCalled: true, wantPositions: positions},
		{name: "trip without positions", positions: []*trip.Position{}, listIsCalled: true, wantPositions: []*trip.Position{}},
		{name: "trip not found never reads positions", tripErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "trip lookup error", tripErr: errDB, wantErr: errDB},
		{name: "positions error", listIsCalled: true, positionsErr: errDB, wantErr: errDB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			trips := NewMocktripRepository(ctrl)
			repo := NewMockpositionRepository(ctrl)
			if tc.tripErr != nil {
				trips.EXPECT().GetByID(gomock.Any(), tripID).Return(nil, tc.tripErr)
			} else {
				trips.EXPECT().GetByID(gomock.Any(), tripID).Return(existing, nil)
			}
			if tc.listIsCalled {
				repo.EXPECT().ListPositions(gomock.Any(), tripID).Return(tc.positions, tc.positionsErr)
			}

			got, err := New(trips, repo).Execute(context.Background(), tripID)

			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantPositions, got)
		})
	}
}
