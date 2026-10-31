package create

import (
	"context"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create

type tripRepository interface {
	Save(ctx context.Context, t *trip.Trip) error
	GetByID(ctx context.Context, id string) (*trip.Trip, error)
}

type idempotencyStore interface {
	Find(ctx context.Context, key string) (model.IdempotencyRecord, bool, error)
	Reserve(ctx context.Context, key, requestHash string) (bool, error)
	Complete(ctx context.Context, key, tripID string) error
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type tripMetrics interface {
	TripCreated(ctx context.Context)
}
