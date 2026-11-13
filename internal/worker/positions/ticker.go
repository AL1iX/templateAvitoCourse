package positions

import "time"

type ticker interface {
	C() <-chan time.Time
	Stop()
}

type timeTicker struct {
	*time.Ticker
}

func (t timeTicker) C() <-chan time.Time {
	return t.Ticker.C
}

func newTimeTicker(d time.Duration) ticker {
	return timeTicker{time.NewTicker(d)}
}
