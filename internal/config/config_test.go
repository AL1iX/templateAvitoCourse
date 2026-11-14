package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	keyCheckInterval = "POSITION_CHECK_INTERVAL"
	keyStaleAfter    = "POSITION_STALE_AFTER"
	keyWorkers       = "POSITION_WORKERS"
	keyQueueSize     = "POSITION_QUEUE_SIZE"
	keyBatchSize     = "POSITION_BATCH_SIZE"
	keyPushURL       = "PUSH_HTTP_URL"
	keyPushTimeout   = "PUSH_REQUEST_TIMEOUT"

	testPushURL = "http://localhost:8090"
)

var requiredEnv = map[string]string{
	"HTTP_ADDR":                  ":8080",
	"LOG_LEVEL":                  "info",
	"SHUTDOWN_TIMEOUT":           "10s",
	"DATABASE_URL":               "postgres://tripgo@localhost:5432/tripgo?sslmode=disable",
	"DATABASE_MAX_CONNS":         "10",
	"DATABASE_MIN_CONNS":         "2",
	"DATABASE_MAX_CONN_LIFETIME": "30m",
	"DATABASE_CONNECT_TIMEOUT":   "5s",
	"DATABASE_QUERY_TIMEOUT":     "3s",
	keyPushURL:                   testPushURL,
}

var optionalKeys = []string{
	"IDEMPOTENCY_KEY_TTL",
	"HTTP_READ_TIMEOUT",
	"HTTP_READ_HEADER_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	keyCheckInterval,
	keyStaleAfter,
	keyWorkers,
	keyQueueSize,
	keyBatchSize,
	keyPushTimeout,
}

func setEnv(t *testing.T, overrides map[string]string) {
	t.Helper()

	for key, value := range requiredEnv {
		t.Setenv(key, value)
	}
	for _, key := range optionalKeys {
		t.Setenv(key, "")
	}
	for key, value := range overrides {
		t.Setenv(key, value)
	}
}

func TestLoadPositionDefaults(t *testing.T) {
	setEnv(t, nil)

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, 30*time.Second, cfg.PositionCheckInterval)
	require.Equal(t, 60*time.Second, cfg.PositionStaleAfter)
	require.Equal(t, 4, cfg.PositionWorkers)
	require.Equal(t, 100, cfg.PositionQueueSize)
	require.Equal(t, 100, cfg.PositionBatchSize)
	require.Equal(t, time.Second, cfg.PushRequestTimeout)
	require.Equal(t, testPushURL, cfg.PushHTTPURL)
}

func TestLoadPositionOverrides(t *testing.T) {
	setEnv(t, map[string]string{
		keyCheckInterval: "2s",
		keyStaleAfter:    "5s",
		keyWorkers:       "8",
		keyQueueSize:     "10",
		keyBatchSize:     "50",
		keyPushTimeout:   "250ms",
		keyPushURL:       "https://push.internal:8443",
	})

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, 2*time.Second, cfg.PositionCheckInterval)
	require.Equal(t, 5*time.Second, cfg.PositionStaleAfter)
	require.Equal(t, 8, cfg.PositionWorkers)
	require.Equal(t, 10, cfg.PositionQueueSize)
	require.Equal(t, 50, cfg.PositionBatchSize)
	require.Equal(t, 250*time.Millisecond, cfg.PushRequestTimeout)
	require.Equal(t, "https://push.internal:8443", cfg.PushHTTPURL)
}

func TestLoadRejectsInvalidPositionSettings(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "check interval is zero", key: keyCheckInterval, value: "0s"},
		{name: "check interval is negative", key: keyCheckInterval, value: "-5s"},
		{name: "check interval is not a duration", key: keyCheckInterval, value: "often"},
		{name: "check interval has a carriage return", key: keyCheckInterval, value: "30s\r"},
		{name: "stale after is zero", key: keyStaleAfter, value: "0s"},
		{name: "stale after has no unit", key: keyStaleAfter, value: "60"},
		{name: "workers is zero", key: keyWorkers, value: "0"},
		{name: "workers is negative", key: keyWorkers, value: "-1"},
		{name: "workers is not a number", key: keyWorkers, value: "four"},
		{name: "queue size is zero", key: keyQueueSize, value: "0"},
		{name: "queue size is not a number", key: keyQueueSize, value: "1.5"},
		{name: "batch size is zero", key: keyBatchSize, value: "0"},
		{name: "batch size is negative", key: keyBatchSize, value: "-100"},
		{name: "batch size is not a number", key: keyBatchSize, value: "many"},
		{name: "push timeout is zero", key: keyPushTimeout, value: "0s"},
		{name: "push timeout has no unit", key: keyPushTimeout, value: "1"},
		{name: "push url is missing", key: keyPushURL, value: ""},
		{name: "push url has no scheme", key: keyPushURL, value: "localhost:23809"},
		{name: "push url has an unsupported scheme", key: keyPushURL, value: "ftp://localhost:21"},
		{name: "push url has no host", key: keyPushURL, value: "http://"},
		{name: "push url is malformed", key: keyPushURL, value: "://localhost"},
		{name: "push url has a query", key: keyPushURL, value: "http://localhost:8090/?x=1"},
		{name: "push url has a fragment", key: keyPushURL, value: "http://localhost:8090/#x"},
		{name: "push url has a user", key: keyPushURL, value: "http://user@localhost:8090"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{tc.key: tc.value})

			_, err := Load()

			require.ErrorContains(t, err, tc.key)
		})
	}
}
