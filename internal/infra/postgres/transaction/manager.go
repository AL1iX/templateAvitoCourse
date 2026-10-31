package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey struct{}

var txKey = ctxKey{}

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Manager struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Manager {
	return &Manager{pool: pool, queryTimeout: queryTimeout}
}

// WithTimeout limits one database operation to the configured query timeout.
// The deadline is derived from ctx, so it never outlives the request or the
// surrounding transaction.
func (m *Manager) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, m.queryTimeout)
}

func (m *Manager) Executor(ctx context.Context) Executor {
	if tx, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return tx
	}
	return m.pool
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, alreadyInTx := ctx.Value(txKey).(pgx.Tx); alreadyInTx {
		return fn(ctx)
	}

	beginCtx, cancelBegin := m.WithTimeout(ctx)
	tx, err := m.pool.BeginTx(beginCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	cancelBegin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = m.rollback(ctx, tx)
			panic(r)
		}
	}()

	if err := fn(context.WithValue(ctx, txKey, tx)); err != nil {
		if rbErr := m.rollback(ctx, tx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			return fmt.Errorf("rollback after %w: %w", err, rbErr)
		}
		return err
	}

	commitCtx, cancelCommit := m.WithTimeout(ctx)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// rollback runs under its own timeout and ignores cancellation of ctx, so a
// cancelled request still releases the transaction.
func (m *Manager) rollback(ctx context.Context, tx pgx.Tx) error {
	rbCtx, cancel := m.WithTimeout(context.WithoutCancel(ctx))
	defer cancel()
	return tx.Rollback(rbCtx)
}
