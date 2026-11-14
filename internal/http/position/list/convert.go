package list

import (
	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

func toAPI(positions []*trip.Position) generated.TripPositionsResponse {
	items := make([]generated.TripPosition, 0, len(positions))
	for _, p := range positions {
		items = append(items, generated.TripPosition{
			Id:          p.ID(),
			TripId:      uuid.MustParse(p.TripID()),
			Coordinates: generated.Coordinates{Latitude: p.Point().Latitude, Longitude: p.Point().Longitude},
			RecordedAt:  p.RecordedAt(),
		})
	}
	return generated.TripPositionsResponse{Positions: items}
}
