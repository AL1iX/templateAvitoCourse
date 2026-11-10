package get

import (
	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

func toAPI(t *trip.Trip) generated.Trip {
	return generated.Trip{
		Id:         uuid.MustParse(t.ID()),
		UserId:     uuid.MustParse(t.UserID()),
		DriverId:   uuid.MustParse(t.DriverID()),
		StartPoint: generated.Coordinates{Latitude: t.Start().Latitude, Longitude: t.Start().Longitude},
		EndPoint:   generated.Coordinates{Latitude: t.End().Latitude, Longitude: t.End().Longitude},
		Price:      t.Price(),
		Status:     generated.TripStatus(t.Status()),
		StartedAt:  t.StartedAt(),
		FinishedAt: t.FinishedAt(),
	}
}
