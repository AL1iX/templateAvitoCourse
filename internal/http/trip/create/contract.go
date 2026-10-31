package create

import (
	"context"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

//go:generate go tool mockgen -source=contract.go -destination=mocks_test.go -package=create

type useCase interface {
	Execute(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, error)
}

type errorWriter interface {
	Write(w http.ResponseWriter, r *http.Request, err error)
}
