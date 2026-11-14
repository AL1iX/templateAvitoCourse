package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AL1iX/templateAvitoCourse/internal/config"
	"github.com/AL1iX/templateAvitoCourse/internal/gateway/push"
	"github.com/AL1iX/templateAvitoCourse/internal/http/domainerror"
	"github.com/AL1iX/templateAvitoCourse/internal/http/health"
	"github.com/AL1iX/templateAvitoCourse/internal/http/middleware"
	listposhttp "github.com/AL1iX/templateAvitoCourse/internal/http/position/list"
	saveposhttp "github.com/AL1iX/templateAvitoCourse/internal/http/position/save"
	"github.com/AL1iX/templateAvitoCourse/internal/http/router"
	createhttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/create"
	finishhttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/finish"
	gethttp "github.com/AL1iX/templateAvitoCourse/internal/http/trip/get"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/postgres/transaction"
	"github.com/AL1iX/templateAvitoCourse/internal/infra/telemetry"
	triprepo "github.com/AL1iX/templateAvitoCourse/internal/repository/trip"
	listposuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/position/list"
	saveposuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/position/save"
	createuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/create"
	finishuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/finish"
	getuc "github.com/AL1iX/templateAvitoCourse/internal/usecase/trip/get"
	"github.com/AL1iX/templateAvitoCourse/internal/worker/positions"
)

const (
	instrumentationName        = "github.com/AL1iX/templateAvitoCourse"
	idempotencyCleanupInterval = time.Hour
)

type Container struct {
	server          *http.Server
	pool            *pgxpool.Pool
	tripRepo        *triprepo.Repository
	pushClient      *push.Client
	poller          *positions.Component
	telemetry       *telemetry.Telemetry
	logger          *slog.Logger
	shutdownTimeout time.Duration

	shutdownDeadline time.Time
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	tel, err := telemetry.New(ctx, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("init telemetry: %w", err)
	}

	c, err := build(ctx, cfg, tel)
	if err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
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

	txManager := transaction.New(pool, cfg.DatabaseQueryTimeout)
	tripRepo := triprepo.New(txManager, cfg.IdempotencyKeyTTL)

	meter := tel.MeterProvider.Meter(instrumentationName)
	err = telemetry.RegisterActiveTrips(meter, tripRepo.CountActive)
	if err != nil {
		pool.Close()
		return nil, err
	}

	pushClient := push.New(cfg.PushHTTPURL, cfg.PushRequestTimeout)
	poller := positions.New(
		positions.Config{
			CheckInterval:  cfg.PositionCheckInterval,
			StaleAfter:     cfg.PositionStaleAfter,
			Workers:        cfg.PositionWorkers,
			QueueSize:      cfg.PositionQueueSize,
			RequestTimeout: cfg.PushRequestTimeout,
		},
		tripRepo, pushClient, m.positions, tel.TracerProvider.Tracer(instrumentationName), logger,
	)
	err = telemetry.RegisterPositionQueue(meter, poller.QueueSize, poller.ActiveWorkers)
	if err != nil {
		pushClient.Close()
		pool.Close()
		return nil, err
	}

	createUC := createuc.New(tripRepo, tripRepo, txManager, m.trips, logger)
	getUC := getuc.New(tripRepo)
	finishUC := finishuc.New(tripRepo, txManager, m.trips, logger)
	savePositionUC := saveposuc.New(tripRepo, txManager, m.trips)
	listPositionsUC := listposuc.New(tripRepo, tripRepo)

	errMapper := domainerror.New(logger)
	handler := router.New(
		createhttp.New(createUC, errMapper),
		gethttp.New(getUC, errMapper),
		finishhttp.New(finishUC, errMapper),
		saveposhttp.New(savePositionUC, poller, errMapper),
		listposhttp.New(listPositionsUC, errMapper),
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
		tripRepo:        tripRepo,
		pushClient:      pushClient,
		poller:          poller,
		telemetry:       tel,
		logger:          logger,
		shutdownTimeout: cfg.ShutdownTimeout,
	}, nil
}

func (c *Container) Run(ctx context.Context) error {
	cleanupCtx, stopCleanup := context.WithCancel(context.WithoutCancel(ctx))
	var cleanup sync.WaitGroup
	cleanup.Add(1)
	go func() {
		defer cleanup.Done()
		c.cleanupExpiredKeys(cleanupCtx)
	}()
	c.poller.Start(ctx)

	errCh := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		c.logger.Info("http server starting", "addr", c.server.Addr)
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var serveErr error
	select {
	case err := <-errCh:
		serveErr = fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		c.logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.shutdownTimeout)
	defer cancel()
	c.shutdownDeadline, _ = shutdownCtx.Deadline()

	return errors.Join(serveErr, c.shutdown(shutdownCtx, serveDone, stopCleanup, &cleanup))
}

func (c *Container) shutdown(
	ctx context.Context, serveDone <-chan struct{}, stopCleanup context.CancelFunc, cleanup *sync.WaitGroup,
) error {
	var errs []error

	if err := c.server.Shutdown(ctx); err != nil {
		c.logger.WarnContext(ctx, "http server did not stop in time, closing connections", "error", err)
		_ = c.server.Close()
		errs = append(errs, fmt.Errorf("graceful shutdown: %w", err))
	} else {
		c.logger.InfoContext(ctx, "http server stopped")
	}
	<-serveDone

	if err := c.poller.Stop(ctx); err != nil {
		errs = append(errs, err)
	}

	stopCleanup()
	cleanup.Wait()

	return errors.Join(errs...)
}

// cleanupExpiredKeys deletes expired idempotency keys at start and then every
// idempotencyCleanupInterval until ctx is cancelled. Correctness does not
// depend on it: expired keys are already ignored on read.
func (c *Container) cleanupExpiredKeys(ctx context.Context) {
	ticker := time.NewTicker(idempotencyCleanupInterval)
	defer ticker.Stop()

	for {
		n, err := c.tripRepo.DeleteExpiredKeys(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			c.logger.Error("idempotency cleanup failed", "error", err)
		case n > 0:
			c.logger.Info("expired idempotency keys deleted", "count", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Container) Close(ctx context.Context) {
	deadline := c.shutdownDeadline
	if deadline.IsZero() {
		deadline = time.Now().Add(c.shutdownTimeout)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	c.pushClient.Close()
	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.logger.Warn("telemetry shutdown failed", "error", err)
	}
	c.pool.Close()
}
