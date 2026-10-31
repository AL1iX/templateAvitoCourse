package idempotency

import (
	"context"
	"errors"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/AL1iX/templateAvitoCourse/internal/txmanager"
)

type Record struct {
	Key          string
	RequestHash  string
	ResponseBody []byte
}

var ErrNotFound = errors.New("idempotency key not found")

// Repository stores idempotency keys. A key is "live" for ttl after its
// created_at; an older row is treated as absent on read and may be reused or
// removed at any time.
type Repository struct {
	tx  *txmanager.TxManager
	ttl time.Duration
}

func NewRepository(tx *txmanager.TxManager, ttl time.Duration) *Repository {
	return &Repository{tx: tx, ttl: ttl}
}

// Get returns the record only while the key is live; an expired key is
// reported as ErrNotFound.
func (r *Repository) Get(ctx context.Context, key string) (Record, error) {
	ctx, cancel := r.tx.WithTimeout(ctx)
	defer cancel()
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Select("key", "request_hash", "response_body").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.Expr("created_at > now() - make_interval(secs => ?)", r.ttl.Seconds())).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return Record{}, err
	}

	var rec Record
	row := exec.QueryRow(ctx, query, args...)
	if err := row.Scan(&rec.Key, &rec.RequestHash, &rec.ResponseBody); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	return rec, nil
}

// Reserve claims the key for the current transaction. It returns false when a
// live key already exists. An expired key is taken over atomically by the
// ON CONFLICT ... DO UPDATE ... WHERE: Postgres locks the conflicting row, so
// of several concurrent requests exactly one wins and the others see a fresh
// (live) row and get false.
func (r *Repository) Reserve(ctx context.Context, key, requestHash string) (bool, error) {
	ctx, cancel := r.tx.WithTimeout(ctx)
	defer cancel()
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Insert("idempotency_keys").
		Columns("key", "request_hash", "response_body").
		Values(key, requestHash, []byte("{}")).
		Suffix(`ON CONFLICT (key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			response_body = EXCLUDED.response_body,
			created_at = now()
			WHERE idempotency_keys.created_at <= now() - make_interval(secs => ?)`, r.ttl.Seconds()).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, err
	}

	tag, err := exec.Exec(ctx, query, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) Fill(ctx context.Context, key string, responseBody []byte) error {
	ctx, cancel := r.tx.WithTimeout(ctx)
	defer cancel()
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Update("idempotency_keys").
		Set("response_body", responseBody).
		Where(sq.Eq{"key": key}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	_, err = exec.Exec(ctx, query, args...)
	return err
}

// DeleteExpired removes keys older than ttl. It is safe to run concurrently
// with Reserve: both lock the row, and a row that was just reused no longer
// matches the condition, so a live key is never deleted.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	ctx, cancel := r.tx.WithTimeout(ctx)
	defer cancel()
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Delete("idempotency_keys").
		Where(sq.Expr("created_at <= now() - make_interval(secs => ?)", r.ttl.Seconds())).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, err
	}

	tag, err := exec.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RunCleanup deletes expired keys immediately and then every interval until
// ctx is cancelled. Correctness does not depend on it: expired keys are
// already ignored on read.
func (r *Repository) RunCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		n, err := r.DeleteExpired(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			logger.Error("idempotency cleanup failed", "error", err)
		case n > 0:
			logger.Info("expired idempotency keys deleted", "count", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
