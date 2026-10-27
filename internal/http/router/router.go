package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AL1iX/templateAvitoCourse/api"
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
	middlewares ...func(http.Handler) http.Handler,
) http.Handler {
	impl := &Router{create: create, get: get, finish: finish, health: health}

	mux := chi.NewRouter()
	api.HandlerFromMux(impl, mux)

	var handler http.Handler = mux
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.WithRoute(r.Context(), routePattern(mux, r))
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func routePattern(mux *chi.Mux, r *http.Request) string {
	rctx := chi.NewRouteContext()
	if mux.Match(rctx, r.Method, r.URL.Path) {
		return rctx.RoutePattern()
	}
	return unmatchedRoute
}

func (rt *Router) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	rt.create.Handle(w, r, params)
}

func (rt *Router) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	rt.get.Handle(w, r, id)
}

func (rt *Router) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	rt.finish.Handle(w, r, id)
}

func (rt *Router) Health(w http.ResponseWriter, r *http.Request) {
	rt.health.Health(w, r)
}

func (rt *Router) Ready(w http.ResponseWriter, r *http.Request) {
	rt.health.Ready(w, r)
}
