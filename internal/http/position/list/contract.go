package list

import (
	"context"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=list

type useCase interface {
	Execute(ctx context.Context, tripID string) ([]*trip.Position, error)
}

type errorWriter interface {
	Write(w http.ResponseWriter, r *http.Request, err error)
}
