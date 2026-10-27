package create

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
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

type mocks struct {
	repo    *MocktripRepository
	idem    *MockidempotencyStore
	tx      *MocktxManager
	metrics *MocktripMetrics
}

func TestExecute(t *testing.T) {
	plain := model.CreateTripInput{
		UserID: "u1", DriverID: "d1",
		Start: trip.Point{Latitude: 59.9, Longitude: 30.3},
		End:   trip.Point{Latitude: 59.9, Longitude: 30.4},
		Price: 100,
	}
	keyed := plain
	keyed.IdempotencyKey = "k1"
	keyed.RequestHash = "h1"
	invalid := plain
	invalid.Price = -1

	existing := trip.Restore("old", "u1", "d1", trip.Point{}, trip.Point{}, 100,
		trip.StatusActive, time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), nil)
	errDB := errors.New("db down")

	runTx := func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	sameRecord := model.IdempotencyRecord{RequestHash: "h1", TripID: "old"}

	tests := []struct {
		name         string
		in           model.CreateTripInput
		setup        func(m mocks)
		wantErr      error
		wantInvalid  bool
		wantReplayed bool
	}{
		{
			name: "trip is created",
			in:   plain,
			setup: func(m mocks) {
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
				m.metrics.EXPECT().TripCreated(gomock.Any())
			},
		},
		{
			name: "driver is busy",
			in:   plain,
			setup: func(m mocks) {
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)
			},
			wantErr: trip.ErrDriverBusy,
		},
		{
			name:        "invalid input never reaches the repository",
			in:          invalid,
			setup:       func(m mocks) {},
			wantInvalid: true,
		},
		{
			name: "repository error",
			in:   plain,
			setup: func(m mocks) {
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "new idempotency key",
			in:   keyed,
			setup: func(m mocks) {
				m.idem.EXPECT().Find(gomock.Any(), "k1").Return(model.IdempotencyRecord{}, false, nil)
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
				m.idem.EXPECT().Reserve(gomock.Any(), "k1", "h1").Return(true, nil)
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
				m.idem.EXPECT().Complete(gomock.Any(), "k1", gomock.Any()).Return(nil)
				m.metrics.EXPECT().TripCreated(gomock.Any())
			},
		},
		{
			name: "same key and same body is a replay",
			in:   keyed,
			setup: func(m mocks) {
				m.idem.EXPECT().Find(gomock.Any(), "k1").Return(sameRecord, true, nil)
				m.repo.EXPECT().GetByID(gomock.Any(), "old").Return(existing, nil)
			},
			wantReplayed: true,
		},
		{
			name: "same key and different body is a conflict",
			in:   keyed,
			setup: func(m mocks) {
				m.idem.EXPECT().Find(gomock.Any(), "k1").
					Return(model.IdempotencyRecord{RequestHash: "other", TripID: "old"}, true, nil)
			},
			wantErr: trip.ErrIdempotencyConflict,
		},
		{
			name: "lost the race for the key and replays the winner",
			in:   keyed,
			setup: func(m mocks) {
				m.idem.EXPECT().Find(gomock.Any(), "k1").Return(model.IdempotencyRecord{}, false, nil)
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
				m.idem.EXPECT().Reserve(gomock.Any(), "k1", "h1").Return(false, nil)
				m.idem.EXPECT().Find(gomock.Any(), "k1").Return(sameRecord, true, nil)
				m.repo.EXPECT().GetByID(gomock.Any(), "old").Return(existing, nil)
			},
			wantReplayed: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{
				repo:    NewMocktripRepository(ctrl),
				idem:    NewMockidempotencyStore(ctrl),
				tx:      NewMocktxManager(ctrl),
				metrics: NewMocktripMetrics(ctrl),
			}
			tc.setup(m)
			logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

			got, err := New(m.repo, m.idem, m.tx, m.metrics, logger).Execute(context.Background(), tc.in)

			switch {
			case tc.wantInvalid:
				var verr *trip.ValidationError
				require.ErrorAs(t, err, &verr)
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			default:
				require.NoError(t, err)
				require.NotNil(t, got.Trip)
				require.Equal(t, tc.wantReplayed, got.Replayed)
			}
		})
	}
}
