package save

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

var point = trip.Point{Latitude: 59.935, Longitude: 30.335}

func TestExecuteCommitFailureDoesNotCountPosition(t *testing.T) {
	errCommit := errors.New("commit failed")
	recordedAt := time.Now().Add(-time.Minute)

	ctrl := gomock.NewController(t)
	repo := NewMockpositionRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	metrics := NewMockpositionMetrics(ctrl)
	repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).
		Return(trip.RestorePosition(42, tripID, point, recordedAt), nil)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context) error) error {
			if err := fn(ctx); err != nil {
				return err
			}
			return errCommit
		},
	)

	got, err := New(repo, tx, metrics).Execute(context.Background(), tripID, point, recordedAt)

	require.ErrorIs(t, err, errCommit)
	require.Nil(t, got)
}

func TestExecute(t *testing.T) {
	recordedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	errDB := errors.New("db down")

	tests := []struct {
		name       string
		point      trip.Point
		recordedAt time.Time
		repoErr    error
		reachesTx  bool
		wantErr    error
		wantField  string
	}{
		{name: "position is saved", point: point, recordedAt: recordedAt, reachesTx: true},
		{name: "zero coordinates are valid", point: trip.Point{}, recordedAt: recordedAt, reachesTx: true},
		{name: "invalid coordinates never reach the repository", point: trip.Point{Latitude: 91}, recordedAt: recordedAt, wantField: "coordinates"},
		{name: "recorded_at too far in the future never reaches the repository", point: point, recordedAt: time.Now().Add(time.Hour), wantField: "recorded_at"},
		{name: "trip not found", point: point, recordedAt: recordedAt, reachesTx: true, repoErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "trip completed", point: point, recordedAt: recordedAt, reachesTx: true, repoErr: trip.ErrTripCompleted, wantErr: trip.ErrTripCompleted},
		{name: "repository error", point: point, recordedAt: recordedAt, reachesTx: true, repoErr: errDB, wantErr: errDB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockpositionRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			metrics := NewMockpositionMetrics(ctrl)
			if tc.reachesTx {
				tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
				)
				repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, p *trip.Position) (*trip.Position, error) {
						require.Equal(t, tripID, p.TripID())
						require.Equal(t, tc.point, p.Point())
						require.True(t, tc.recordedAt.Equal(p.RecordedAt()))
						if tc.repoErr != nil {
							return nil, tc.repoErr
						}
						return trip.RestorePosition(42, p.TripID(), p.Point(), p.RecordedAt()), nil
					},
				)
			}
			if tc.reachesTx && tc.repoErr == nil {
				metrics.EXPECT().PositionSaved(gomock.Any())
			}

			got, err := New(repo, tx, metrics).Execute(context.Background(), tripID, tc.point, tc.recordedAt)

			switch {
			case tc.wantField != "":
				var verr *trip.ValidationError
				require.ErrorAs(t, err, &verr)
				require.Equal(t, tc.wantField, verr.Field)
				require.Nil(t, got)
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
			default:
				require.NoError(t, err)
				require.Equal(t, int64(42), got.ID())
				require.Equal(t, tripID, got.TripID())
			}
		})
	}
}
