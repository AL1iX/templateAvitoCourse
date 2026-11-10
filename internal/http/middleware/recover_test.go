package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
)

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	errw := domainerror.New(slog.New(slog.NewJSONHandler(&logs, nil)))
	h := Recover(errw)(panicHandler())
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() {
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil))
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"internal_error"`)
	require.NotContains(t, rec.Body.String(), "secret")
	require.Contains(t, logs.String(), "panic: boom: secret")
	require.Contains(t, logs.String(), "goroutine")
}

func TestRecoverKeepsAbortHandler(t *testing.T) {
	errw := domainerror.New(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	h := Recover(errw)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}
