package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

const (
	lastActivity = "COALESCE(last_position_at, started_at)"
	activeOnly   = "status = 'active'"
)

func (r *Repository) FindStale(
	ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleCursor,
) ([]model.StaleTrip, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	builder := sq.Select("id", "driver_id", lastActivity).
		From("trips").
		Where(activeOnly).
		Where(sq.Expr(lastActivity+" < now() - make_interval(secs => ?)", staleAfter.Seconds()))
	if after != nil {
		builder = builder.Where(sq.Expr("("+lastActivity+", id) > (?, ?)", after.LastActivityAt, after.TripID))
	}
	query, args, err := builder.
		OrderBy(lastActivity+" ASC", "id ASC").
		Suffix("LIMIT ?", limit).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select stale trips: %w", err)
	}

	rows, err := r.db.Executor(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()

	trips := make([]model.StaleTrip, 0)
	for rows.Next() {
		var t model.StaleTrip
		if err := rows.Scan(&t.TripID, &t.DriverID, &t.LastActivityAt); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		trips = append(trips, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}
	return trips, nil
}
