package trip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

type idempotencyBody struct {
	TripID string `json:"trip_id"`
}

// Find returns the record only while the key is live; an expired key is
// reported as not found.
func (r *Repository) Find(ctx context.Context, key string) (model.IdempotencyRecord, bool, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Select("request_hash", "response_body").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.Expr("created_at > now() - make_interval(secs => ?)", r.keyTTL.Seconds())).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return model.IdempotencyRecord{}, false, fmt.Errorf("build select idempotency: %w", err)
	}

	var (
		hash string
		body []byte
	)
	if err := r.db.Executor(ctx).QueryRow(ctx, query, args...).Scan(&hash, &body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.IdempotencyRecord{}, false, nil
		}
		return model.IdempotencyRecord{}, false, fmt.Errorf("select idempotency: %w", err)
	}

	var parsed idempotencyBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return model.IdempotencyRecord{}, false, fmt.Errorf("decode idempotency body: %w", err)
	}
	if parsed.TripID == "" {
		return model.IdempotencyRecord{}, false, errors.New("idempotency record has no trip id")
	}
	return model.IdempotencyRecord{RequestHash: hash, TripID: parsed.TripID}, true, nil
}

// Reserve claims the key for the current transaction. It returns false when a
// live key already exists. An expired key is taken over atomically by the
// ON CONFLICT ... DO UPDATE ... WHERE: Postgres locks the conflicting row, so
// of several concurrent requests exactly one wins and the others see a fresh
// (live) row and get false.
func (r *Repository) Reserve(ctx context.Context, key, requestHash string) (bool, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Insert("idempotency_keys").
		Columns("key", "request_hash", "response_body").
		Values(key, requestHash, []byte("{}")).
		Suffix(`ON CONFLICT (key) DO UPDATE SET
			request_hash = EXCLUDED.request_hash,
			response_body = EXCLUDED.response_body,
			created_at = now()
			WHERE idempotency_keys.created_at <= now() - make_interval(secs => ?)`, r.keyTTL.Seconds()).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build reserve idempotency: %w", err)
	}

	tag, err := r.db.Executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("reserve idempotency: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) Complete(ctx context.Context, key, tripID string) error {
	body, err := json.Marshal(idempotencyBody{TripID: tripID})
	if err != nil {
		return fmt.Errorf("encode idempotency body: %w", err)
	}

	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Update("idempotency_keys").
		Set("response_body", body).
		Where(sq.Eq{"key": key}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build complete idempotency: %w", err)
	}

	if _, err := r.db.Executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("complete idempotency: %w", err)
	}
	return nil
}

// DeleteExpiredKeys removes keys older than the TTL and returns how many were
// deleted. It is safe to run concurrently with Reserve: both lock the row, and
// a row that was just reused no longer matches the condition, so a live key
// is never deleted.
func (r *Repository) DeleteExpiredKeys(ctx context.Context) (int64, error) {
	ctx, cancel := r.db.WithTimeout(ctx)
	defer cancel()

	query, args, err := sq.Delete("idempotency_keys").
		Where(sq.Expr("created_at <= now() - make_interval(secs => ?)", r.keyTTL.Seconds())).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build delete expired idempotency: %w", err)
	}

	tag, err := r.db.Executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency: %w", err)
	}
	return tag.RowsAffected(), nil
}
