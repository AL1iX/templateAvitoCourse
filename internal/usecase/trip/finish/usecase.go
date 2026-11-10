package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

type UseCase struct {
	repo    tripRepository
	tx      txManager
	metrics tripMetrics
	logger  *slog.Logger
}

func New(repo tripRepository, tx txManager, metrics tripMetrics, logger *slog.Logger) *UseCase {
	return &UseCase{repo: repo, tx: tx, metrics: metrics, logger: logger}
}

func (u *UseCase) Execute(ctx context.Context, id string) (*trip.Trip, error) {
	var finished *trip.Trip
	err := u.tx.Do(ctx, func(ctx context.Context) error {
		t, err := u.repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := t.Finish(time.Now().UTC().Truncate(time.Microsecond)); err != nil {
			return err
		}
		if err := u.repo.Update(ctx, t); err != nil {
			return err
		}
		finished = t
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("finish trip: %w", err)
	}

	u.metrics.TripCompleted(ctx)
	u.logger.InfoContext(ctx, "trip finished", "trip_id", id)
	return finished, nil
}
