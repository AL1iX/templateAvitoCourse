package positions

import (
	"context"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=positions

type staleTripFinder interface {
	FindStale(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, tripID, driverID string) error
}

type positionMetrics interface {
	StaleCheck(ctx context.Context, result string)
	PositionRequest(ctx context.Context, result string)
	PositionRequestDuration(ctx context.Context, result string, d time.Duration)
}
