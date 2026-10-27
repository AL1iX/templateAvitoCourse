package model

import "github.com/AL1iX/templateAvitoCourse/internal/domain/trip"

type CreateTripInput struct {
	UserID         string
	DriverID       string
	Start          trip.Point
	End            trip.Point
	Price          int64
	IdempotencyKey string
	RequestHash    string
}

type CreateTripResult struct {
	Trip     *trip.Trip
	Replayed bool
}

type IdempotencyRecord struct {
	RequestHash string
	TripID      string
}
