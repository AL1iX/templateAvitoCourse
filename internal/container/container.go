package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AL1iX/templateAvitoCourse/internal/config"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
	"github.com/AL1iX/templateAvitoCourse/internal/http/health"
	"github.com/AL1iX/templateAvitoCourse/internal/http/middleware"
	"github.com/AL1iX/templateAvitoCourse/internal/http/router"
	createhttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/create"
	finishhttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/finish"
	gethttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/get"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres/transaction"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/telemetry"
	triprepo "github.com/AL1iX/templateAvitoCourse/internal/repository/trip"
	createuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/create"
	finishuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/finish"
	getuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/get"
)

const instrumentationName = "github.com/AL1iX/templateAvitoCourse"

type Container struct {
	server          *http.Server
	pool            *pgxpool.Pool
	telemetry       *telemetry.Telemetry
	logger          *slog.Logger
	shutdownTimeout time.Duration
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	tel, err := telemetry.New(ctx, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("init telemetry: %w", err)
	}

	c, err := build(ctx, cfg, tel)
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = tel.Shutdown(shutdownCtx)
		return nil, err
	}
	return c, nil
}

func build(ctx context.Context, cfg config.Config, tel *telemetry.Telemetry) (*Container, error) {
	logger := tel.Logger

	m, err := newMetrics(tel)
	if err != nil {
		return nil, err
	}

	pool, err := postgres.NewPool(ctx, cfg, m.queryTracer)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	txManager := transaction.New(pool)
	tripRepo := triprepo.New(txManager)

	err = telemetry.RegisterActiveTrips(tel.MeterProvider.Meter(instrumentationName),
		func(ctx context.Context) (int64, error) {
			ctx, cancel := context.WithTimeout(ctx, cfg.DatabaseQueryTimeout)
			defer cancel()
			return tripRepo.CountActive(ctx)
		})
	if err != nil {
		pool.Close()
		return nil, err
	}

	createUC := createuc.New(tripRepo, tripRepo, txManager, m.trips, logger)
	getUC := getuc.New(tripRepo)
	finishUC := finishuc.New(tripRepo, txManager, m.trips, logger)

	errMapper := domainerror.New(logger)
	handler := router.New(
		createhttp.New(createUC, errMapper),
		gethttp.New(getUC, errMapper),
		finishhttp.New(finishUC, errMapper),
		health.New(pool, cfg.DatabaseConnectTimeout, logger),
		errMapper,
		middleware.Recover(errMapper),
		middleware.Tracing(tel.TracerProvider, tel.Propagator),
		middleware.Logging(logger),
		m.httpMiddle,
	)

	return &Container{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTPReadTimeout,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
		},
		pool:            pool,
		telemetry:       tel,
		logger:          logger,
		shutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

func (c *Container) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		c.logger.Info("http server starting", "addr", c.server.Addr)
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}
	c.logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
	defer cancel()

	if err := c.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	c.logger.Info("http server stopped")
	return nil
}

func (c *Container) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
	defer cancel()

	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.logger.Warn("telemetry shutdown failed", "error", err)
	}
	c.pool.Close()
}
