package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AL1iX/templateAvitoCourse/internal/config"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/httpapi"
	"github.com/AL1iX/templateAvitoCourse/internal/idempotency"
	"github.com/AL1iX/templateAvitoCourse/internal/postgres"
	"github.com/AL1iX/templateAvitoCourse/internal/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/txmanager"
	"github.com/go-chi/chi/v5"
)

const idempotencyCleanupInterval = time.Hour

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	tripTxManager := txmanager.New(pool, cfg.DatabaseQueryTimeout)
	tripRepo := trip.NewRepository(tripTxManager)
	idemRepo := idempotency.NewRepository(tripTxManager, cfg.IdempotencyKeyTTL)

	go idemRepo.RunCleanup(ctx, idempotencyCleanupInterval, logger)

	handler := httpapi.NewHandler(pool, logger, cfg, tripTxManager, tripRepo, idemRepo)

	router := chi.NewRouter()
	apiHandler := generated.HandlerWithOptions(handler, generated.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: handler.BadParameter,
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiHandler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func newLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}
