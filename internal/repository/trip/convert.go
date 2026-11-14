package trip

import (
	"time"

	domain "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

type tripRow struct {
	ID             string
	UserID         string
	DriverID       string
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
	LastPositionAt *time.Time
}

func (r tripRow) toDomain() *domain.Trip {
	var finishedAt *time.Time
	if r.FinishedAt != nil {
		t := r.FinishedAt.UTC()
		finishedAt = &t
	}

	var lastPositionAt *time.Time
	if r.LastPositionAt != nil {
		t := r.LastPositionAt.UTC()
		lastPositionAt = &t
	}

	return domain.Restore(
		r.ID, r.UserID, r.DriverID,
		domain.Point{Latitude: r.StartLatitude, Longitude: r.StartLongitude},
		domain.Point{Latitude: r.EndLatitude, Longitude: r.EndLongitude},
		r.Price, domain.Status(r.Status), r.StartedAt.UTC(), finishedAt, lastPositionAt,
	)
}
