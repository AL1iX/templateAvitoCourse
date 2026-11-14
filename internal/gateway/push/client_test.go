package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	tripID   = "11111111-1111-4111-8111-111111111111"
	driverID = "33333333-3333-4333-8333-333333333333"

	acceptedBody = `{"message_id":"m1","accepted_at":"2026-11-14T12:00:00Z"}`
)

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func respondWith(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}

func TestRequestPositionSendsContractRequest(t *testing.T) {
	type received struct {
		method      string
		path        string
		contentType string
		body        map[string]any
	}
	got := make(chan received, 1)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		got <- received{method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, acceptedBody)
	})

	err := New(srv.URL, time.Second, propagation.TraceContext{}).RequestPosition(t.Context(), tripID, driverID)

	require.NoError(t, err)
	req := <-got
	require.Equal(t, http.MethodPost, req.method)
	require.Equal(t, "/api/v1/push", req.path)
	require.Equal(t, "application/json", req.contentType)
	requestID, ok := req.body["request_id"].(string)
	require.True(t, ok, "request_id is required by the contract")
	_, parseErr := uuid.Parse(requestID)
	require.NoError(t, parseErr)
	delete(req.body, "request_id")
	require.Equal(t, map[string]any{
		"recipient_id": driverID,
		"kind":         "REQUEST_POSITION",
		"data":         map[string]any{"trip_id": tripID},
	}, req.body, "exactly the fields of the contract, the stub rejects unknown ones")
}

func TestRequestPositionPropagatesTraceContext(t *testing.T) {
	var traceparent string
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("Traceparent")
		w.WriteHeader(http.StatusAccepted)
	})
	ctx, span := sdktrace.NewTracerProvider().Tracer("test").Start(t.Context(), "push")
	defer span.End()

	err := New(srv.URL, time.Second, propagation.TraceContext{}).RequestPosition(ctx, tripID, driverID)

	require.NoError(t, err)
	require.Contains(t, traceparent, span.SpanContext().TraceID().String())
	require.Contains(t, traceparent, span.SpanContext().SpanID().String())
}

func TestRequestPositionTrailingSlashInBaseURL(t *testing.T) {
	paths := make(chan string, 1)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	})

	err := New(srv.URL+"/", time.Second, propagation.TraceContext{}).RequestPosition(t.Context(), tripID, driverID)

	require.NoError(t, err)
	require.Equal(t, "/api/v1/push", <-paths)
}

func TestRequestPositionClassifiesStatuses(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		wantErr      error
		wantAnyError bool
	}{
		{name: "accepted", status: http.StatusAccepted},
		{name: "bad request", status: http.StatusBadRequest, wantErr: ErrRejected},
		{name: "not found", status: http.StatusNotFound, wantErr: ErrRejected},
		{name: "rate limited", status: http.StatusTooManyRequests, wantErr: ErrRejected},
		{name: "internal error", status: http.StatusInternalServerError, wantErr: ErrUnavailable},
		{name: "service unavailable", status: http.StatusServiceUnavailable, wantErr: ErrUnavailable},
		{name: "ok instead of accepted", status: http.StatusOK, wantAnyError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, respondWith(tc.status))

			err := New(srv.URL, time.Second, propagation.TraceContext{}).RequestPosition(t.Context(), tripID, driverID)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantAnyError:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrTransport)
				require.NotErrorIs(t, err, ErrRejected)
				require.NotErrorIs(t, err, ErrUnavailable)
			default:
				require.NoError(t, err)
			}
			if tc.wantErr != nil || tc.wantAnyError {
				require.NotErrorIs(t, err, ErrTransport, "a status code is not a transport error")
			}
		})
	}
}

func TestRequestPositionTransportError(t *testing.T) {
	srv := httptest.NewServer(respondWith(http.StatusAccepted))
	url := srv.URL
	srv.Close()

	err := New(url, time.Second, propagation.TraceContext{}).RequestPosition(t.Context(), tripID, driverID)

	require.ErrorIs(t, err, ErrTransport)
	require.NotErrorIs(t, err, ErrRejected)
	require.NotErrorIs(t, err, ErrUnavailable)
}

func TestRequestPositionContextCancelled(t *testing.T) {
	started := make(chan struct{})
	srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- New(srv.URL, time.Minute, propagation.TraceContext{}).RequestPosition(ctx, tripID, driverID)
	}()

	<-started
	cancel()
	err := <-result

	require.ErrorIs(t, err, ErrTransport)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRequestPositionClientTimeout(t *testing.T) {
	srv := newServer(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})

	err := New(srv.URL, 20*time.Millisecond, propagation.TraceContext{}).RequestPosition(t.Context(), tripID, driverID)

	require.ErrorIs(t, err, ErrTransport)
	var netErr net.Error
	require.True(t, errors.As(err, &netErr))
	require.True(t, netErr.Timeout())
}

type trackingBody struct{ closed bool }

func (b *trackingBody) Read([]byte) (int, error) { return 0, io.EOF }

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResponseBodyIsAlwaysClosed(t *testing.T) {
	for _, status := range []int{
		http.StatusAccepted, http.StatusOK, http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackingBody{}
			c := New("http://push.test", time.Second, propagation.TraceContext{})
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: body}, nil
			})

			_ = c.RequestPosition(t.Context(), tripID, driverID)

			require.True(t, body.closed)
		})
	}
}

func TestNewDerivesAllTimeoutsFromOneSetting(t *testing.T) {
	const timeout = 1500 * time.Millisecond

	c := New("http://push.test", timeout, propagation.TraceContext{})

	require.NotSame(t, http.DefaultClient, c.http)
	require.Equal(t, timeout, c.http.Timeout)
	transport, ok := c.http.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, timeout, transport.TLSHandshakeTimeout)
	require.Equal(t, timeout, transport.ResponseHeaderTimeout)
	require.Positive(t, transport.IdleConnTimeout, "idle connections expire, as in the default transport")
}
