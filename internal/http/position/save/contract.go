package save

import (
	"context"
	"net/http"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=save

type useCase interface {
	Execute(ctx context.Context, tripID string, point trip.Point, recordedAt time.Time) (*trip.Position, error)
}

type errorWriter interface {
	Write(w http.ResponseWriter, r *http.Request, err error)
}
