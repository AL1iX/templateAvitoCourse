package router

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/http/middleware"
)

const unmatchedRoute = "unmatched"

type Router struct {
	create createHandler
	get    tripHandler
	finish tripHandler
	health healthHandler
}

func New(
	create createHandler,
	get, finish tripHandler,
	health healthHandler,
	errw errorWriter,
	middlewares ...func(http.Handler) http.Handler,
) http.Handler {
	impl := &Router{create: create, get: get, finish: finish, health: health}

	mux := chi.NewRouter()
	generated.HandlerWithOptions(impl, generated.ChiServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			errw.Write(w, r, badParam(err))
		},
	})

	var handler http.Handler = mux
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.WithRoute(r.Context(), routePattern(mux, r))
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func badParam(err error) error {
	verr := &trip.ValidationError{Field: "request", Reason: "invalid parameter"}

	var invalid *generated.InvalidParamFormatError
	if errors.As(err, &invalid) {
		verr.Field = invalid.ParamName
		verr.Reason = "invalid format"
	}
	return verr
}

func routePattern(mux *chi.Mux, r *http.Request) string {
	rctx := chi.NewRouteContext()
	if mux.Match(rctx, r.Method, r.URL.Path) {
		return rctx.RoutePattern()
	}
	return unmatchedRoute
}

func (rt *Router) CreateTrip(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	rt.create.Handle(w, r, params)
}

func (rt *Router) GetTrip(w http.ResponseWriter, r *http.Request, id generated.TripId) {
	rt.get.Handle(w, r, id)
}

func (rt *Router) FinishTrip(w http.ResponseWriter, r *http.Request, id generated.TripId) {
	rt.finish.Handle(w, r, id)
}

func (rt *Router) Health(w http.ResponseWriter, r *http.Request) {
	rt.health.Health(w, r)
}

func (rt *Router) Ready(w http.ResponseWriter, r *http.Request) {
	rt.health.Ready(w, r)
}
