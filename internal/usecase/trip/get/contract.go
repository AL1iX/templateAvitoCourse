package get

import (
	"context"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=get

type tripRepository interface {
	GetByID(ctx context.Context, id string) (*trip.Trip, error)
}
