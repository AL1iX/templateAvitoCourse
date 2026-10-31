package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogging(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantLevel  string
		wantPanic  bool
	}{
		{"created", statusHandler(http.StatusCreated), http.StatusCreated, levelInfo, false},
		{"not found", statusHandler(http.StatusNotFound), http.StatusNotFound, levelInfo, false},
		{"server error", statusHandler(http.StatusInternalServerError), http.StatusInternalServerError, levelError, false},
		{
			"implicit ok",
			func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) },
			http.StatusOK, levelInfo, false,
		},
		{panicLabel, panicHandler(), http.StatusInternalServerError, levelError, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := Logging(slog.New(slog.NewJSONHandler(&logs, nil)))(tc.handler)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil).
				WithContext(WithRoute(t.Context(), testRoute))

			serve := func() { h.ServeHTTP(httptest.NewRecorder(), req) }
			if tc.wantPanic {
				require.Panics(t, serve)
			} else {
				serve()
			}

			require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("\n")), "exactly one record per request")
			var entry map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
			require.Equal(t, "request handled", entry["msg"])
			require.Equal(t, tc.wantLevel, entry["level"])
			require.EqualValues(t, tc.wantStatus, entry["status"])
			require.Equal(t, http.MethodPost, entry["method"])
			require.Equal(t, testRoute, entry["route"])
			require.Contains(t, entry, "duration_ms")
		})
	}
}
