package save

import (
	"context"
	"fmt"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

type UseCase struct {
	repo    positionRepository
	tx      txManager
	metrics positionMetrics
}

func New(repo positionRepository, tx txManager, metrics positionMetrics) *UseCase {
	return &UseCase{repo: repo, tx: tx, metrics: metrics}
}

func (u *UseCase) Execute(ctx context.Context, tripID string, point trip.Point, recordedAt time.Time) (*trip.Position, error) {
	p, err := trip.NewPosition(tripID, point, recordedAt, time.Now())
	if err != nil {
		return nil, fmt.Errorf("save position: %w", err)
	}

	var saved *trip.Position
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		saved, err = u.repo.SavePosition(ctx, p)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("save position: %w", err)
	}

	u.metrics.PositionSaved(ctx)
	return saved, nil
}
