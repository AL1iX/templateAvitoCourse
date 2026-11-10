package router_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
	"github.com/AL1iX/templateAvitoCourse/internal/http/router"
)

const tripsPath = "/api/v1/trips"

func TestInvalidParameters(t *testing.T) {
	handler := router.New(nil, nil, nil, nil,
		domainerror.New(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	tests := []struct {
		name   string
		method string
		path   string
		header map[string]string
	}{
		{name: "get with invalid trip id", method: http.MethodGet, path: tripsPath + "/not-a-uuid"},
		{name: "finish with invalid trip id", method: http.MethodPost, path: tripsPath + "/not-a-uuid/finish"},
		{name: "create with invalid idempotency key", method: http.MethodPost, path: tripsPath, header: map[string]string{"Idempotency-Key": "nope"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

			var problem generated.Problem
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&problem))
			require.Equal(t, "invalid_request", problem.Code)
		})
	}
}
