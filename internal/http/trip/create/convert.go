package create

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

const bodyField = "body"

// request mirrors generated.TripData with pointer fields so that a missing or
// null field can be told apart from an explicit zero value.
type request struct {
	UserID     *uuid.UUID `json:"user_id"`
	DriverID   *uuid.UUID `json:"driver_id"`
	StartPoint *point     `json:"start_point"`
	EndPoint   *point     `json:"end_point"`
	Price      *int64     `json:"price"`
}

type point struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// tripData returns the request as generated.TripData; ok is false when any
// required field is missing or null.
func (q request) tripData() (generated.TripData, bool) {
	if q.UserID == nil || q.DriverID == nil || q.Price == nil ||
		q.StartPoint == nil || q.StartPoint.Latitude == nil || q.StartPoint.Longitude == nil ||
		q.EndPoint == nil || q.EndPoint.Latitude == nil || q.EndPoint.Longitude == nil {
		return generated.TripData{}, false
	}
	return generated.TripData{
		UserId:     *q.UserID,
		DriverId:   *q.DriverID,
		StartPoint: generated.Coordinates{Latitude: *q.StartPoint.Latitude, Longitude: *q.StartPoint.Longitude},
		EndPoint:   generated.Coordinates{Latitude: *q.EndPoint.Latitude, Longitude: *q.EndPoint.Longitude},
		Price:      *q.Price,
	}, true
}

func toInput(raw []byte, params generated.CreateTripParams) (model.CreateTripInput, error) {
	var req request
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return model.CreateTripInput{}, &trip.ValidationError{Field: bodyField, Reason: err.Error()}
	}
	// Anything after the first JSON object other than whitespace is an error.
	if err := dec.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return model.CreateTripInput{}, &trip.ValidationError{Field: bodyField, Reason: "unexpected data after JSON object"}
	}
	body, ok := req.tripData()
	if !ok {
		return model.CreateTripInput{}, &trip.ValidationError{
			Field: bodyField, Reason: "user_id, driver_id, start_point, end_point and price are required",
		}
	}

	in := model.CreateTripInput{
		UserID:   body.UserId.String(),
		DriverID: body.DriverId.String(),
		Start:    trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
		End:      trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
		Price:    body.Price,
	}
	if params.IdempotencyKey != nil {
		sum := sha256.Sum256(raw)
		in.IdempotencyKey = params.IdempotencyKey.String()
		in.RequestHash = hex.EncodeToString(sum[:])
	}
	return in, nil
}

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
