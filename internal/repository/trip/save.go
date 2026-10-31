package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

const (
	uniqueViolationCode = "23505"
	activeTripIndex     = "trips_one_active_per_driver_idx"
)

// Save and Update run two statements (the trip and its status history); each
// statement gets its own query timeout.
func (r *Repository) Save(ctx context.Context, t *domain.Trip) error {
	if err := r.insertTrip(ctx, t); err != nil {
		return err
	}
	return r.addHistory(ctx, t.ID(), nil, domain.StatusActive, "trip created")
}

func (r *Repository) insertTrip(ctx context.Context, t *domain.Trip) error {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at").
		Values(t.ID(), t.UserID(), t.DriverID(), t.Start().Latitude, t.Start().Longitude,
			t.End().Latitude, t.End().Longitude, t.Price(), string(t.Status()), t.StartedAt()).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	if _, err := r.db.Executor(ctx).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == activeTripIndex {
			return domain.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, t *domain.Trip) error {
	if err := r.updateTrip(ctx, t); err != nil {
		return err
	}
	from := domain.StatusActive
	return r.addHistory(ctx, t.ID(), &from, t.Status(), "trip finished")
}

func (r *Repository) updateTrip(ctx context.Context, t *domain.Trip) error {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Update("trips").
		Set("status", string(t.Status())).
		Set("finished_at", t.FinishedAt()).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": t.ID()}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update trip: %w", err)
	}

	tag, err := r.db.Executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update trip: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTripNotFound
	}
	return nil
}

func (r *Repository) addHistory(ctx context.Context, tripID string, from *domain.Status, to domain.Status, reason string) error {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	var fromVal any
	if from != nil {
		fromVal = string(*from)
	}

	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromVal, string(to), reason).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert history: %w", err)
	}

	if _, err := r.db.Executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert history: %w", err)
	}
	return nil
}
