package positions

import (
	"context"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=positions

type staleTripFinder interface {
	FindStale(
		ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleCursor,
	) ([]model.StaleTrip, error)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, tripID, driverID string) error
}

type positionMetrics interface {
	StaleCheck(ctx context.Context, result string)
	StaleScanRows(ctx context.Context, rows int)
	PositionRequest(ctx context.Context, result string)
	PositionRequestDuration(ctx context.Context, result string, d time.Duration)
}
