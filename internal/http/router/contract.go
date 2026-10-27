package router

import (
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

type createHandler interface {
	Handle(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams)
}

type tripHandler interface {
	Handle(w http.ResponseWriter, r *http.Request, id generated.TripId)
}

type errorWriter interface {
	Write(w http.ResponseWriter, r *http.Request, err error)
}

type healthHandler interface {
	Health(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}
