package create

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/AL1iX/templateAvitoCourse/api"
	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

func toInput(raw []byte, params api.CreateTripParams) (model.CreateTripInput, error) {
	var body api.TripData
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return model.CreateTripInput{}, &trip.ValidationError{Field: "body", Reason: err.Error()}
	}
	if body.UserId == uuid.Nil || body.DriverId == uuid.Nil {
		return model.CreateTripInput{}, &trip.ValidationError{Field: "user_id, driver_id", Reason: "must not be empty"}
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
