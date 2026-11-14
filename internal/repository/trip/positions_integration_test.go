//go:build integration

package trip_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres/transaction"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
	triprepo "github.com/AL1iX/templateAvitoCourse/internal/repository/trip"
)

var positionPoint = domain.Point{Latitude: 59.93, Longitude: 30.33}

func (f fixture) newSavedTrip(t *testing.T, startedAt time.Time) *domain.Trip {
	t.Helper()

	tr, err := domain.New(uuid.NewString(), uuid.NewString(), uuid.NewString(),
		domain.Point{Latitude: 56.8, Longitude: 60.6}, domain.Point{Latitude: 56.9, Longitude: 60.7},
		500, startedAt)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := f.pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", tr.ID())
		require.NoError(t, err)
	})
	require.NoError(t, f.repo.Save(t.Context(), tr))
	return tr
}

func (f fixture) savePosition(ctx context.Context, tripID string, recordedAt time.Time) (*domain.Position, error) {
	p, err := domain.NewPosition(tripID, positionPoint, recordedAt, now())
	if err != nil {
		return nil, err
	}

	var saved *domain.Position
	err = f.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		saved, err = f.repo.SavePosition(ctx, p)
		return err
	})
	return saved, err
}

func (f fixture) mustSavePosition(t *testing.T, tripID string, recordedAt time.Time) *domain.Position {
	t.Helper()

	p, err := f.savePosition(t.Context(), tripID, recordedAt)
	require.NoError(t, err)
	return p
}

func (f fixture) lastPositionAt(t *testing.T, tripID string) *time.Time {
	t.Helper()

	got, err := f.repo.GetByID(t.Context(), tripID)
	require.NoError(t, err)
	return got.LastPositionAt()
}

func (f fixture) positionCount(t *testing.T, tripID string) int {
	t.Helper()

	var n int
	err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM trip_positions WHERE trip_id = $1", tripID).Scan(&n)
	require.NoError(t, err)
	return n
}

func requireSameTime(t *testing.T, want time.Time, got *time.Time) {
	t.Helper()

	require.NotNil(t, got)
	require.True(t, want.Equal(*got), "want %s, got %s", want, *got)
	require.Equal(t, time.UTC, got.Location(), "the API serializes the time in UTC")
}

func TestSavePositionStoresPointAndUpdatesLastPosition(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())
	require.Nil(t, f.lastPositionAt(t, tr.ID()))
	recordedAt := now().Add(-5 * time.Second)

	saved := f.mustSavePosition(t, tr.ID(), recordedAt)

	require.Positive(t, saved.ID())
	require.Equal(t, tr.ID(), saved.TripID())
	require.Equal(t, positionPoint, saved.Point())
	requireSameTime(t, recordedAt, f.lastPositionAt(t, tr.ID()))

	got, err := f.repo.ListPositions(t.Context(), tr.ID())
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, saved.ID(), got[0].ID())
	require.Equal(t, positionPoint, got[0].Point())
	require.True(t, recordedAt.Equal(got[0].RecordedAt()))
}

func TestSavePositionOutOfOrderDoesNotAgeTrip(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())
	latest := now().Add(-10 * time.Second)

	f.mustSavePosition(t, tr.ID(), latest)
	f.mustSavePosition(t, tr.ID(), latest.Add(-time.Hour))

	requireSameTime(t, latest, f.lastPositionAt(t, tr.ID()))
	require.Equal(t, 2, f.positionCount(t, tr.ID()), "the late position is stored, only last_position_at is protected")

	newer := latest.Add(5 * time.Second)
	f.mustSavePosition(t, tr.ID(), newer)

	requireSameTime(t, newer, f.lastPositionAt(t, tr.ID()))
}

func TestSavePositionTripNotFound(t *testing.T) {
	f := setup(t)

	_, err := f.savePosition(t.Context(), uuid.NewString(), now())

	require.ErrorIs(t, err, domain.ErrTripNotFound)
}

func TestSavePositionCompletedTrip(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())
	require.NoError(t, f.finish(t.Context(), tr.ID()))

	_, err := f.savePosition(t.Context(), tr.ID(), now())

	require.ErrorIs(t, err, domain.ErrTripCompleted)
	require.Equal(t, 0, f.positionCount(t, tr.ID()))
	require.Nil(t, f.lastPositionAt(t, tr.ID()))
}

func TestSavePositionRollback(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())
	errBoom := errors.New("boom")
	p, err := domain.NewPosition(tr.ID(), positionPoint, now(), now())
	require.NoError(t, err)

	err = f.tx.Do(t.Context(), func(ctx context.Context) error {
		if _, err := f.repo.SavePosition(ctx, p); err != nil {
			return err
		}
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 0, f.positionCount(t, tr.ID()))
	require.Nil(t, f.lastPositionAt(t, tr.ID()), "last_position_at is rolled back together with the insert")
}

func TestListPositionsSortedByRecordedAtThenID(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())
	base := now().Add(-time.Hour)

	first := f.mustSavePosition(t, tr.ID(), base.Add(2*time.Second))
	second := f.mustSavePosition(t, tr.ID(), base)
	third := f.mustSavePosition(t, tr.ID(), base.Add(2*time.Second))

	got, err := f.repo.ListPositions(t.Context(), tr.ID())

	require.NoError(t, err)
	ids := make([]int64, 0, len(got))
	for _, p := range got {
		ids = append(ids, p.ID())
	}
	require.Equal(t, []int64{second.ID(), first.ID(), third.ID()}, ids)
}

func TestListPositionsEmpty(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now())

	got, err := f.repo.ListPositions(t.Context(), tr.ID())

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

func (f fixture) staleAmong(t *testing.T, ids ...string) []model.StaleTrip {
	t.Helper()

	all, err := f.repo.FindStale(t.Context(), time.Minute)
	require.NoError(t, err)

	var res []model.StaleTrip
	for _, s := range all {
		if slices.Contains(ids, s.TripID) {
			res = append(res, s)
		}
	}
	return res
}

func TestFindStale(t *testing.T) {
	f := setup(t)
	silentSinceStart := f.newSavedTrip(t, now().Add(-10*time.Minute))
	silentSincePosition := f.newSavedTrip(t, now().Add(-time.Hour))
	f.mustSavePosition(t, silentSincePosition.ID(), now().Add(-5*time.Minute))
	freshNoPositions := f.newSavedTrip(t, now())
	freshWithPosition := f.newSavedTrip(t, now().Add(-time.Hour))
	f.mustSavePosition(t, freshWithPosition.ID(), now().Add(-10*time.Second))
	completed := f.newSavedTrip(t, now().Add(-time.Hour))
	require.NoError(t, f.finish(t.Context(), completed.ID()))

	got := f.staleAmong(t,
		silentSinceStart.ID(), silentSincePosition.ID(), freshNoPositions.ID(), freshWithPosition.ID(), completed.ID())

	require.Equal(t, []model.StaleTrip{
		{TripID: silentSinceStart.ID(), DriverID: silentSinceStart.DriverID()},
		{TripID: silentSincePosition.ID(), DriverID: silentSincePosition.DriverID()},
	}, got, "only active trips silent for more than a minute, the longest-silent first")
}

func TestFindStaleOutOfOrderPositionDoesNotMakeTripStale(t *testing.T) {
	f := setup(t)
	tr := f.newSavedTrip(t, now().Add(-time.Hour))

	f.mustSavePosition(t, tr.ID(), now().Add(-10*time.Second))
	f.mustSavePosition(t, tr.ID(), now().Add(-30*time.Minute))

	require.Empty(t, f.staleAmong(t, tr.ID()))
}

type expiringTimeout struct {
	inner  *transaction.Manager
	failAt int
	calls  int
}

func (s *expiringTimeout) Executor(ctx context.Context) transaction.Executor {
	return s.inner.Executor(ctx)
}

func (s *expiringTimeout) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	s.calls++
	if s.calls == s.failAt {
		return context.WithDeadline(ctx, time.Unix(0, 0))
	}
	return s.inner.WithTimeout(ctx)
}

func requireTimeoutOnEveryStatement(
	t *testing.T, f fixture, statements int, call func(*triprepo.Repository) error, wantErr error,
) {
	t.Helper()

	for failAt := 1; failAt <= statements; failAt++ {
		spy := &expiringTimeout{inner: f.tx, failAt: failAt}

		err := call(triprepo.New(spy, keyTTL))

		require.ErrorIs(t, err, context.DeadlineExceeded, "statement %d must run under WithTimeout", failAt)
	}

	spy := &expiringTimeout{inner: f.tx, failAt: statements + 1}
	err := call(triprepo.New(spy, keyTTL))
	if wantErr == nil {
		require.NoError(t, err)
	} else {
		require.ErrorIs(t, err, wantErr)
	}
	require.Equal(t, statements, spy.calls, "number of statements with a timeout")
}

func TestPositionStatementsApplyQueryTimeout(t *testing.T) {
	f := setup(t)
	active := f.newSavedTrip(t, now())
	completed := f.newSavedTrip(t, now())
	require.NoError(t, f.finish(t.Context(), completed.ID()))
	save := func(tripID string) func(*triprepo.Repository) error {
		return func(repo *triprepo.Repository) error {
			p, err := domain.NewPosition(tripID, positionPoint, now(), now())
			require.NoError(t, err)
			_, err = repo.SavePosition(t.Context(), p)
			return err
		}
	}

	t.Run("save position", func(t *testing.T) {
		requireTimeoutOnEveryStatement(t, f, 2, save(active.ID()), nil)
	})
	t.Run("save position of a completed trip", func(t *testing.T) {
		requireTimeoutOnEveryStatement(t, f, 2, save(completed.ID()), domain.ErrTripCompleted)
	})
	t.Run("list positions", func(t *testing.T) {
		requireTimeoutOnEveryStatement(t, f, 1, func(repo *triprepo.Repository) error {
			_, err := repo.ListPositions(t.Context(), active.ID())
			return err
		}, nil)
	})
	t.Run("find stale", func(t *testing.T) {
		requireTimeoutOnEveryStatement(t, f, 1, func(repo *triprepo.Repository) error {
			_, err := repo.FindStale(t.Context(), time.Minute)
			return err
		}, nil)
	})
}
