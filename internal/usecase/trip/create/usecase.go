package create

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

var errKeyTaken = errors.New("idempotency key is taken by a concurrent request")

type UseCase struct {
	repo    tripRepository
	idem    idempotencyStore
	tx      txManager
	metrics tripMetrics
	logger  *slog.Logger
}

func New(repo tripRepository, idem idempotencyStore, tx txManager, metrics tripMetrics, logger *slog.Logger) *UseCase {
	return &UseCase{repo: repo, idem: idem, tx: tx, metrics: metrics, logger: logger}
}

func (u *UseCase) Execute(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
	if in.IdempotencyKey != "" {
		res, found, err := u.replay(ctx, in)
		if err != nil || found {
			return res, err
		}
	}

	t, err := trip.New(uuid.NewString(), in.UserID, in.DriverID, in.Start, in.End, in.Price, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return model.CreateTripResult{}, fmt.Errorf("create trip: %w", err)
	}

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if in.IdempotencyKey != "" {
			reserved, err := u.idem.Reserve(ctx, in.IdempotencyKey, in.RequestHash)
			if err != nil {
				return err
			}
			if !reserved {
				return errKeyTaken
			}
		}
		if err := u.repo.Save(ctx, t); err != nil {
			return err
		}
		if in.IdempotencyKey != "" {
			return u.idem.Complete(ctx, in.IdempotencyKey, t.ID())
		}
		return nil
	})
	if errors.Is(err, errKeyTaken) {
		res, found, rerr := u.replay(ctx, in)
		if rerr != nil {
			return model.CreateTripResult{}, rerr
		}
		if !found {
			return model.CreateTripResult{}, fmt.Errorf("create trip: %w", errKeyTaken)
		}
		return res, nil
	}
	if err != nil {
		return model.CreateTripResult{}, fmt.Errorf("create trip: %w", err)
	}

	u.metrics.TripCreated(ctx)
	u.logger.InfoContext(ctx, "trip created", "trip_id", t.ID())
	return model.CreateTripResult{Trip: t}, nil
}

func (u *UseCase) replay(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, bool, error) {
	rec, found, err := u.idem.Find(ctx, in.IdempotencyKey)
	if err != nil {
		return model.CreateTripResult{}, false, fmt.Errorf("find idempotency key: %w", err)
	}
	if !found {
		return model.CreateTripResult{}, false, nil
	}
	if rec.RequestHash != in.RequestHash {
		return model.CreateTripResult{}, true, trip.ErrIdempotencyConflict
	}
	t, err := u.repo.GetByID(ctx, rec.TripID)
	if err != nil {
		return model.CreateTripResult{}, true, fmt.Errorf("load replayed trip: %w", err)
	}
	return model.CreateTripResult{Trip: t, Replayed: true}, true, nil
}
