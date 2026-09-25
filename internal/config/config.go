package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration

	HTTPReadTimeout       time.Duration
	HTTPReadHeaderTimeout time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
}

func Load() (Config, error) {
	var cfg Config

	httpAddr, err := requireString("HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}
	cfg.HTTPAddr = httpAddr

	cfg.LogLevel, err = requireLogLevel("LOG_LEVEL")
	if err != nil {
		return Config{}, err
	}

	cfg.ShutdownTimeout, err = requireDuration("SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPReadTimeout, err = optionalDuration("HTTP_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPReadHeaderTimeout, err = optionalDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPWriteTimeout, err = optionalDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg.HTTPIdleTimeout, err = optionalDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}

	databaseURL, err := requireString("DATABASE_URL")
	if err != nil {
		return Config{}, err
	}
	cfg.DatabaseURL = databaseURL

	cfg.DatabaseMaxConns, err = requireInt32("DATABASE_MAX_CONNS")
	if err != nil {
		return Config{}, err
	}

	cfg.DatabaseMinConns, err = requireInt32("DATABASE_MIN_CONNS")
	if err != nil {
		return Config{}, err
	}

	cfg.DatabaseMaxConnLifetime, err = requireDuration("DATABASE_MAX_CONN_LIFETIME")
	if err != nil {
		return Config{}, err
	}

	cfg.DatabaseConnectTimeout, err = requireDuration("DATABASE_CONNECT_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	cfg.DatabaseQueryTimeout, err = requireDuration("DATABASE_QUERY_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	if cfg.DatabaseMinConns > cfg.DatabaseMaxConns {
		return Config{}, fmt.Errorf("DATABASE_MIN_CONNS (%d) must not exceed DATABASE_MAX_CONNS (%d)",
			cfg.DatabaseMinConns, cfg.DatabaseMaxConns)
	}
	return cfg, nil
}

func requireString(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required environment variable %s not set", key)
	}
	return v, nil
}

func requireLogLevel(key string) (slog.Level, error) {
	v, err := requireString(key)
	if err != nil {
		return 0, err
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid log level %q: %w", key, v, err)
	}
	return lvl, nil
}

func requireDuration(key string) (time.Duration, error) {
	v, err := requireString(key)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid duration %q: %w", key, v, err)
	}
	return d, nil
}

func optionalDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid duration %q: %w", key, v, err)
	}
	return d, nil
}

func requireInt32(key string) (int32, error) {
	v, err := requireString(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s: invalid integer %q: %w", key, v, err)
	}
	return int32(n), nil
}
