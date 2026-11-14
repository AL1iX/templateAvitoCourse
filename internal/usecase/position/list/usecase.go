package list

import (
	"context"
	"fmt"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

type UseCase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *UseCase {
	return &UseCase{trips: trips, positions: positions}
}

func (u *UseCase) Execute(ctx context.Context, tripID string) ([]*trip.Position, error) {
	if _, err := u.trips.GetByID(ctx, tripID); err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	positions, err := u.positions.ListPositions(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return positions, nil
}
