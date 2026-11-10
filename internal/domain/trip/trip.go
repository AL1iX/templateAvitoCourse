package trip

import "time"

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	id         string
	userID     string
	driverID   string
	start      Point
	end        Point
	price      int64
	status     Status
	startedAt  time.Time
	finishedAt *time.Time
}

func New(id, userID, driverID string, start, end Point, price int64, startedAt time.Time) (*Trip, error) {
	if userID == "" || driverID == "" {
		return nil, &ValidationError{Field: "user_id, driver_id", Reason: "are required"}
	}
	if !validPoint(start) {
		return nil, &ValidationError{Field: "start_point", Reason: "coordinates out of range"}
	}
	if !validPoint(end) {
		return nil, &ValidationError{Field: "end_point", Reason: "coordinates out of range"}
	}
	if price < 0 {
		return nil, &ValidationError{Field: "price", Reason: "must not be negative"}
	}
	return &Trip{
		id: id, userID: userID, driverID: driverID,
		start: start, end: end, price: price,
		status: StatusActive, startedAt: startedAt,
	}, nil
}

func Restore(id, userID, driverID string, start, end Point, price int64, status Status, startedAt time.Time, finishedAt *time.Time) *Trip {
	return &Trip{
		id: id, userID: userID, driverID: driverID,
		start: start, end: end, price: price,
		status: status, startedAt: startedAt, finishedAt: finishedAt,
	}
}

func (t *Trip) Finish(at time.Time) error {
	if t.status != StatusActive {
		return ErrTripCompleted
	}
	t.status = StatusCompleted
	t.finishedAt = &at
	return nil
}

func (t *Trip) ID() string             { return t.id }
func (t *Trip) UserID() string         { return t.userID }
func (t *Trip) DriverID() string       { return t.driverID }
func (t *Trip) Start() Point           { return t.start }
func (t *Trip) End() Point             { return t.end }
func (t *Trip) Price() int64           { return t.price }
func (t *Trip) Status() Status         { return t.status }
func (t *Trip) StartedAt() time.Time   { return t.startedAt }
func (t *Trip) FinishedAt() *time.Time { return t.finishedAt }

func validPoint(p Point) bool {
	return p.Latitude >= -90 && p.Latitude <= 90 && p.Longitude >= -180 && p.Longitude <= 180
}
