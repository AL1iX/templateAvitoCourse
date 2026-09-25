package idempotency

import (
	"context"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AL1iX/templateAvitoCourse/internal/txmanager"
)

type Record struct {
	Key          string
	RequestHash  string
	ResponseBody []byte
}

var ErrNotFound = errors.New("idempotency key not found")

const uniqueViolationCode = "23505"

type Repository struct {
	tx *txmanager.TxManager
}

func NewRepository(tx *txmanager.TxManager) *Repository {
	return &Repository{tx: tx}
}

func (r *Repository) Get(ctx context.Context, key string) (Record, error) {
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Select("key", "request_hash", "response_body").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
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

// Reserve claims key for this request. It returns false, without error, when
// another request already claimed the same key: the caller should abort its
// own transaction and look up that request's stored response instead.
func (r *Repository) Reserve(ctx context.Context, key, requestHash string) (bool, error) {
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Insert("idempotency_keys").
		Columns("key", "request_hash", "response_body").
		Values(key, requestHash, []byte("{}")).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, err
	}

	if _, err := exec.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) Fill(ctx context.Context, key string, responseBody []byte) error {
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
