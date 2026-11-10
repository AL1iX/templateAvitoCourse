package middleware

import "net/http"

const (
	testRoute  = "/api/v1/trips/{tripId}"
	levelInfo  = "INFO"
	levelError = "ERROR"
	panicLabel = "panic"
)

func statusHandler(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

func panicHandler() http.HandlerFunc {
	return func(http.ResponseWriter, *http.Request) {
		panic("boom: secret")
	}
}
