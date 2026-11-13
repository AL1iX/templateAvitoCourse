package save

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

const bodyField = "body"

type request struct {
	Coordinates *point     `json:"coordinates"`
	RecordedAt  *time.Time `json:"recorded_at"`
}

type point struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func decode(r io.Reader) (trip.Point, time.Time, error) {
	var req request
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return trip.Point{}, time.Time{}, &trip.ValidationError{Field: bodyField, Reason: err.Error()}
	}
	if err := dec.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return trip.Point{}, time.Time{}, &trip.ValidationError{Field: bodyField, Reason: "unexpected data after JSON object"}
	}
	if req.Coordinates == nil || req.Coordinates.Latitude == nil || req.Coordinates.Longitude == nil || req.RecordedAt == nil {
		return trip.Point{}, time.Time{}, &trip.ValidationError{
			Field: bodyField, Reason: "coordinates and recorded_at are required",
		}
	}
	point := trip.Point{Latitude: *req.Coordinates.Latitude, Longitude: *req.Coordinates.Longitude}
	return point, *req.RecordedAt, nil
}

func toAPI(p *trip.Position) generated.TripPosition {
	return generated.TripPosition{
		Id:          p.ID(),
		TripId:      uuid.MustParse(p.TripID()),
		Coordinates: generated.Coordinates{Latitude: p.Point().Latitude, Longitude: p.Point().Longitude},
		RecordedAt:  p.RecordedAt(),
	}
}
