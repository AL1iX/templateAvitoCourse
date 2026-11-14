package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func (r *Repository) SavePosition(ctx context.Context, p *domain.Position) (*domain.Position, error) {
	touched, err := r.touchLastPosition(ctx, p.TripID(), p.RecordedAt())
	if err != nil {
		return nil, err
	}
	if !touched {
		return nil, r.notActiveError(ctx, p.TripID())
	}

	id, err := r.insertPosition(ctx, p)
	if err != nil {
		return nil, err
	}
	return domain.RestorePosition(id, p.TripID(), p.Point(), p.RecordedAt()), nil
}

func (r *Repository) touchLastPosition(ctx context.Context, tripID string, recordedAt time.Time) (bool, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Update("trips").
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": tripID}).
		Where(activeOnly).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build update last position: %w", err)
	}

	tag, err := r.db.Executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("update last position: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) notActiveError(ctx context.Context, tripID string) error {
	t, err := r.GetByID(ctx, tripID)
	if err != nil {
		return err
	}
	if t.Status() == domain.StatusActive {
		return fmt.Errorf("trip %s is active but its last position was not updated", tripID)
	}
	return domain.ErrTripCompleted
}

func (r *Repository) insertPosition(ctx context.Context, p *domain.Position) (int64, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID(), p.Point().Latitude, p.Point().Longitude, p.RecordedAt()).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build insert position: %w", err)
	}

	var id int64
	if err := r.db.Executor(ctx).QueryRow(ctx, query, args...).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert position: %w", err)
	}
	return id, nil
}

func (r *Repository) ListPositions(ctx context.Context, tripID string) ([]*domain.Position, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Select("id", "trip_id", "latitude", "longitude", "recorded_at").
		From("trip_positions").
		Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at ASC", "id ASC").
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select positions: %w", err)
	}

	rows, err := r.db.Executor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}
	defer rows.Close()

	positions := make([]*domain.Position, 0)
	for rows.Next() {
		var (
			id         int64
			owner      string
			point      domain.Point
			recordedAt time.Time
		)
		if err := rows.Scan(&id, &owner, &point.Latitude, &point.Longitude, &recordedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, domain.RestorePosition(id, owner, point, recordedAt.UTC()))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}
