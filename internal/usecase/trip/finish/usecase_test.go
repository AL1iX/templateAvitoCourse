package finish

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func TestExecute(t *testing.T) {
	startedAt := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Hour)
	active := func() *trip.Trip {
		return trip.Restore("t1", "u1", "d1", trip.Point{}, trip.Point{}, 100, trip.StatusActive, startedAt, nil)
	}
	completed := func() *trip.Trip {
		return trip.Restore("t1", "u1", "d1", trip.Point{}, trip.Point{}, 100, trip.StatusCompleted, startedAt, &completedAt)
	}
	errDB := errors.New("db down")

	tests := []struct {
		name    string
		setup   func(repo *MocktripRepository, metrics *MocktripMetrics)
		wantErr error
	}{
		{
			name: "active trip is finished",
			setup: func(repo *MocktripRepository, metrics *MocktripMetrics) {
				repo.EXPECT().GetForUpdate(gomock.Any(), "t1").Return(active(), nil)
				repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)
				metrics.EXPECT().TripCompleted(gomock.Any())
			},
		},
		{
			name: "trip not found",
			setup: func(repo *MocktripRepository, _ *MocktripMetrics) {
				repo.EXPECT().GetForUpdate(gomock.Any(), "t1").Return(nil, trip.ErrTripNotFound)
			},
			wantErr: trip.ErrTripNotFound,
		},
		{
			name: "trip already completed",
			setup: func(repo *MocktripRepository, _ *MocktripMetrics) {
				repo.EXPECT().GetForUpdate(gomock.Any(), "t1").Return(completed(), nil)
			},
			wantErr: trip.ErrTripCompleted,
		},
		{
			name: "update fails",
			setup: func(repo *MocktripRepository, _ *MocktripMetrics) {
				repo.EXPECT().GetForUpdate(gomock.Any(), "t1").Return(active(), nil)
				repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			metrics := NewMocktripMetrics(ctrl)
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
			)
			tc.setup(repo, metrics)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

			got, err := New(repo, tx, metrics, logger).Execute(context.Background(), "t1")

			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantErr != nil {
				require.Nil(t, got)
				return
			}
			require.Equal(t, trip.StatusCompleted, got.Status())
			require.NotNil(t, got.FinishedAt())
		})
	}
}
