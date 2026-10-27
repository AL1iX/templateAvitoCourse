package finish

import (
	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/api"
	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
)

func toAPI(t *trip.Trip) api.Trip {
	return api.Trip{
		Id:         uuid.MustParse(t.ID()),
		UserId:     uuid.MustParse(t.UserID()),
		DriverId:   uuid.MustParse(t.DriverID()),
		StartPoint: api.Coordinates{Latitude: t.Start().Latitude, Longitude: t.Start().Longitude},
		EndPoint:   api.Coordinates{Latitude: t.End().Latitude, Longitude: t.End().Longitude},
		Price:      t.Price(),
		Status:     api.TripStatus(t.Status()),
		StartedAt:  t.StartedAt(),
		FinishedAt: t.FinishedAt(),
	}
}
