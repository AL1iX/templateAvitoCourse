package health

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/api"
)

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database is up", nil, http.StatusOK, string(api.Ok)},
		{"database is down", errors.New("connection refused"), http.StatusServiceUnavailable, string(api.Unavailable)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := NewMockpinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).Return(tc.pingErr)
			h := New(db, time.Second, slog.New(slog.NewJSONHandler(io.Discard, nil)))

			rec := httptest.NewRecorder()
			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			require.Equal(t, tc.wantStatus, rec.Code)
			require.JSONEq(t, `{"status":"`+tc.wantBody+`"}`, rec.Body.String())
		})
	}
}
