package trip

import (
	"context"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AL1iX/templateAvitoCourse/internal/txmanager"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Trip struct {
	ID             string
	UserID         string
	DriverID       string
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         Status
	StartedAt      time.Time
	FinishedAt     *time.Time
}

var (
	ErrNotFound   = errors.New("trip not found")
	ErrCompleted  = errors.New("trip already completed")
	ErrDriverBusy = errors.New("driver already has an active trip")
)

const uniqueViolationCode = "23505"

type Repository struct {
	tx *txmanager.TxManager
}

func NewRepository(tx *txmanager.TxManager) *Repository {
	return &Repository{tx: tx}
}

func (r *Repository) Create(ctx context.Context, t Trip) error {
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at").
		Values(t.ID, t.UserID, t.DriverID, t.StartLatitude, t.StartLongitude,
			t.EndLatitude, t.EndLongitude, t.Price, string(StatusActive), t.StartedAt).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	if _, err := exec.Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return ErrDriverBusy
		}
		return err
	}
	return nil
}

func (r *Repository) AddStatusHistory(ctx context.Context, tripID string, from *Status, to Status, reason string) error {
	exec := r.tx.Executor(ctx)

	var fromVal any
	if from != nil {
		fromVal = string(*from)
	}

	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromVal, string(to), reason).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	_, err = exec.Exec(ctx, query, args...)
	return err
}

func (r *Repository) GetByID(ctx context.Context, id string) (Trip, error) {
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Select(
		"id", "user_id", "driver_id",
		"start_latitude", "start_longitude", "end_latitude", "end_longitude",
		"price", "status", "started_at", "finished_at",
	).From("trips").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return Trip{}, err
	}

	var t Trip
	row := exec.QueryRow(ctx, query, args...)
	err = row.Scan(&t.ID, &t.UserID, &t.DriverID,
		&t.StartLatitude, &t.StartLongitude, &t.EndLatitude, &t.EndLongitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Trip{}, ErrNotFound
		}
		return Trip{}, err
	}
	return t, nil
}

func (r *Repository) Finish(ctx context.Context, id string, finishedAt time.Time) error {
	exec := r.tx.Executor(ctx)

	query, args, err := sq.Update("trips").
		Set("status", string(StatusCompleted)).
		Set("finished_at", finishedAt).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id, "status": string(StatusActive)}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	tag, err := exec.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, getErr := r.GetByID(ctx, id); errors.Is(getErr, ErrNotFound) {
			return ErrNotFound
		} else if getErr != nil {
			return getErr
		}
		return ErrCompleted
	}
	return nil
}
