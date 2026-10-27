//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres/transaction"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
	triprepo "github.com/AL1iX/templateAvitoCourse/internal/repository/trip"
)

type fixture struct {
	pool *pgxpool.Pool
	tx   *transaction.Manager
	repo *triprepo.Repository
}

func setup(t *testing.T) fixture {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	require.NotEmpty(t, url, "DATABASE_URL must be set, run: set -a; source .env; set +a")

	pool, err := pgxpool.New(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, pool.Ping(t.Context()))

	tx := transaction.New(pool)
	return fixture{pool: pool, tx: tx, repo: triprepo.New(tx)}
}

func (f fixture) newTrip(t *testing.T, driverID string) *domain.Trip {
	t.Helper()

	tr, err := domain.New(uuid.NewString(), uuid.NewString(), driverID,
		domain.Point{Latitude: 56.8, Longitude: 60.6}, domain.Point{Latitude: 56.9, Longitude: 60.7},
		500, now())
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := f.pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", tr.ID())
		require.NoError(t, err)
	})
	return tr
}

func (f fixture) historyCount(t *testing.T, tripID string) int {
	t.Helper()

	var n int
	err := f.pool.QueryRow(t.Context(),
		"SELECT count(*) FROM trip_status_history WHERE trip_id = $1", tripID).Scan(&n)
	require.NoError(t, err)
	return n
}

func (f fixture) finish(ctx context.Context, id string) error {
	return f.tx.Do(ctx, func(ctx context.Context) error {
		t, err := f.repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := t.Finish(now()); err != nil {
			return err
		}
		return f.repo.Update(ctx, t)
	})
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func TestSaveAndGetByID(t *testing.T) {
	f := setup(t)
	want := f.newTrip(t, uuid.NewString())

	require.NoError(t, f.repo.Save(t.Context(), want))

	got, err := f.repo.GetByID(t.Context(), want.ID())
	require.NoError(t, err)
	require.Equal(t, want.ID(), got.ID())
	require.Equal(t, want.DriverID(), got.DriverID())
	require.Equal(t, want.Start(), got.Start())
	require.Equal(t, domain.StatusActive, got.Status())
	require.True(t, want.StartedAt().Equal(got.StartedAt()))
	require.Nil(t, got.FinishedAt())
	require.Equal(t, 1, f.historyCount(t, want.ID()))
}

func TestGetByIDNotFound(t *testing.T) {
	f := setup(t)

	_, err := f.repo.GetByID(t.Context(), uuid.NewString())

	require.ErrorIs(t, err, domain.ErrTripNotFound)
}

func TestSaveDriverBusy(t *testing.T) {
	f := setup(t)
	driverID := uuid.NewString()
	first := f.newTrip(t, driverID)
	second := f.newTrip(t, driverID)
	require.NoError(t, f.repo.Save(t.Context(), first))

	err := f.repo.Save(t.Context(), second)

	require.ErrorIs(t, err, domain.ErrDriverBusy)
	require.Equal(t, 0, f.historyCount(t, second.ID()))

	require.NoError(t, f.finish(t.Context(), first.ID()))
	require.NoError(t, f.repo.Save(t.Context(), second), "driver is free again after the trip is finished")
}

func TestFinishUpdatesTripAndHistory(t *testing.T) {
	f := setup(t)
	tr := f.newTrip(t, uuid.NewString())
	require.NoError(t, f.repo.Save(t.Context(), tr))

	require.NoError(t, f.finish(t.Context(), tr.ID()))

	got, err := f.repo.GetByID(t.Context(), tr.ID())
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, got.Status())
	require.NotNil(t, got.FinishedAt())
	require.Equal(t, 2, f.historyCount(t, tr.ID()))
	require.ErrorIs(t, f.finish(t.Context(), tr.ID()), domain.ErrTripCompleted)
}

func TestConcurrentFinishCompletesOnce(t *testing.T) {
	f := setup(t)
	tr := f.newTrip(t, uuid.NewString())
	require.NoError(t, f.repo.Save(t.Context(), tr))

	const workers = 5
	results := make(chan error, workers)
	for range workers {
		go func() { results <- f.finish(t.Context(), tr.ID()) }()
	}

	var finished, alreadyCompleted int
	for range workers {
		err := <-results
		switch {
		case err == nil:
			finished++
		case errors.Is(err, domain.ErrTripCompleted):
			alreadyCompleted++
		default:
			require.NoError(t, err)
		}
	}

	require.Equal(t, 1, finished)
	require.Equal(t, workers-1, alreadyCompleted)
	require.Equal(t, 2, f.historyCount(t, tr.ID()))
}

func TestTransactionRollback(t *testing.T) {
	f := setup(t)
	tr := f.newTrip(t, uuid.NewString())
	errBoom := errors.New("boom")

	err := f.tx.Do(t.Context(), func(ctx context.Context) error {
		if err := f.repo.Save(ctx, tr); err != nil {
			return err
		}
		if _, err := f.repo.GetByID(ctx, tr.ID()); err != nil {
			return err
		}
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	_, getErr := f.repo.GetByID(t.Context(), tr.ID())
	require.ErrorIs(t, getErr, domain.ErrTripNotFound)
	require.Equal(t, 0, f.historyCount(t, tr.ID()))
}

func TestIdempotencyStore(t *testing.T) {
	f := setup(t)
	key := uuid.NewString()
	t.Cleanup(func() {
		_, err := f.pool.Exec(context.Background(), "DELETE FROM idempotency_keys WHERE key = $1", key)
		require.NoError(t, err)
	})

	_, found, err := f.repo.Find(t.Context(), key)
	require.NoError(t, err)
	require.False(t, found)

	reserved, err := f.repo.Reserve(t.Context(), key, "hash-1")
	require.NoError(t, err)
	require.True(t, reserved)

	reserved, err = f.repo.Reserve(t.Context(), key, "hash-2")
	require.NoError(t, err)
	require.False(t, reserved)

	tripID := uuid.NewString()
	require.NoError(t, f.repo.Complete(t.Context(), key, tripID))

	rec, found, err := f.repo.Find(t.Context(), key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, model.IdempotencyRecord{RequestHash: "hash-1", TripID: tripID}, rec)
}

func TestCountActive(t *testing.T) {
	f := setup(t)

	before, err := f.repo.CountActive(t.Context())
	require.NoError(t, err)

	tr := f.newTrip(t, uuid.NewString())
	require.NoError(t, f.repo.Save(t.Context(), tr))

	withTrip, err := f.repo.CountActive(t.Context())
	require.NoError(t, err)
	require.Equal(t, before+1, withTrip)

	require.NoError(t, f.finish(t.Context(), tr.ID()))

	afterFinish, err := f.repo.CountActive(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, afterFinish)
}
