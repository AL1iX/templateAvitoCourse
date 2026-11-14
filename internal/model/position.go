package model

import "time"

type StaleTrip struct {
	TripID         string
	DriverID       string
	LastActivityAt time.Time
}

type StaleCursor struct {
	LastActivityAt time.Time
	TripID         string
}
