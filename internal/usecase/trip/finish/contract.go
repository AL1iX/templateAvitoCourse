package finish

import (
	"context"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=finish

type tripRepository interface {
	GetForUpdate(ctx context.Context, id string) (*trip.Trip, error)
	Update(ctx context.Context, t *trip.Trip) error
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type tripMetrics interface {
	TripCompleted(ctx context.Context)
}
