package save

import (
	"context"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=save

type positionRepository interface {
	SavePosition(ctx context.Context, p *trip.Position) (*trip.Position, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type positionMetrics interface {
	PositionSaved(ctx context.Context)
}
