//go:build integration

package container_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/AL1iX/templateAvitoCourse/internal/config"
	"github.com/AL1iX/templateAvitoCourse/internal/container"
)

const (
	shutdownBound        = 30 * time.Second
	unreachableCollector = "http://127.0.0.1:1"
	neverExpiring        = "1000000h"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestShutdownLeavesNoGoroutines(t *testing.T) {
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", unreachableCollector)
	t.Setenv("IDEMPOTENCY_KEY_TTL", neverExpiring)
	cfg, err := config.Load()
	require.NoError(t, err, "DATABASE_URL and the other settings come from .env, run make test-integration")
	ctx, stop := context.WithCancel(t.Context())
	app, err := container.New(ctx, cfg)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()

	stop()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(shutdownBound):
		t.Fatal("Run did not return after the shutdown signal")
	}
	app.Close(t.Context())
}
