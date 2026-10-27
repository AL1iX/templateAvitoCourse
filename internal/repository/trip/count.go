package trip

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	query, args, err := sq.Select("count(*)").From("trips").
		Where(sq.Eq{"status": string(domain.StatusActive)}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}

	var n int64
	if err := r.db.Executor(ctx).QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return n, nil
}
