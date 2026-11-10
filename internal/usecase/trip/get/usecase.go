package get

import (
	"context"
	"fmt"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
}

func New(repo tripRepository) *UseCase {
	return &UseCase{repo: repo}
}

func (u *UseCase) Execute(ctx context.Context, id string) (*trip.Trip, error) {
	t, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}
	return t, nil
}
