package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at",
}

func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Trip, error) {
	return r.fetch(ctx, id, false)
}

func (r *Repository) GetForUpdate(ctx context.Context, id string) (*domain.Trip, error) {
	return r.fetch(ctx, id, true)
}

func (r *Repository) fetch(ctx context.Context, id string, lock bool) (*domain.Trip, error) {
	builder := sq.Select(tripColumns...).From("trips").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)
	if lock {
		builder = builder.Suffix("FOR UPDATE")
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select trip: %w", err)
	}

	var row tripRow
	err = r.db.Executor(ctx).QueryRow(ctx, query, args...).Scan(
		&row.ID, &row.UserID, &row.DriverID,
		&row.StartLatitude, &row.StartLongitude, &row.EndLatitude, &row.EndLongitude,
		&row.Price, &row.Status, &row.StartedAt, &row.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTripNotFound
		}
		return nil, fmt.Errorf("select trip: %w", err)
	}
	return row.toDomain(), nil
}
