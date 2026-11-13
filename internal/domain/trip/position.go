package trip

import "time"

const maxFutureSkew = time.Minute

type Position struct {
	id         int64
	tripID     string
	point      Point
	recordedAt time.Time
}

func NewPosition(tripID string, point Point, recordedAt, now time.Time) (*Position, error) {
	if !validPoint(point) {
		return nil, &ValidationError{Field: "coordinates", Reason: "out of range"}
	}
	if recordedAt.After(now.Add(maxFutureSkew)) {
		return nil, &ValidationError{Field: "recorded_at", Reason: "must not be more than a minute in the future"}
	}
	return &Position{
		tripID:     tripID,
		point:      point,
		recordedAt: recordedAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func RestorePosition(id int64, tripID string, point Point, recordedAt time.Time) *Position {
	return &Position{id: id, tripID: tripID, point: point, recordedAt: recordedAt}
}

func (p *Position) ID() int64             { return p.id }
func (p *Position) TripID() string        { return p.tripID }
func (p *Position) Point() Point          { return p.point }
func (p *Position) RecordedAt() time.Time { return p.recordedAt }
