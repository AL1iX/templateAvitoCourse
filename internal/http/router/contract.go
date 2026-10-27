package router

import (
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/api"
)

type createHandler interface {
	Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams)
}

type tripHandler interface {
	Handle(w http.ResponseWriter, r *http.Request, id api.TripId)
}

type healthHandler interface {
	Health(w http.ResponseWriter, r *http.Request)
	Ready(w http.ResponseWriter, r *http.Request)
}
