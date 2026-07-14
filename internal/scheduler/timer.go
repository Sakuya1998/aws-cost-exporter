package scheduler

import "time"

// Timer is a resettable delay used by the runner scheduling loop.
type Timer interface {
	Chan() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

// systemTimer wraps a standard library timer for production scheduling.
type systemTimer struct {
	timer *time.Timer
}

func newSystemTimer(delay time.Duration) Timer {
	return &systemTimer{timer: time.NewTimer(delay)}
}

// NewSystemTimer constructs a production scheduling timer.
func NewSystemTimer(delay time.Duration) Timer { return newSystemTimer(delay) }

func (timer *systemTimer) Chan() <-chan time.Time { return timer.timer.C }
func (timer *systemTimer) Stop() bool           { return timer.timer.Stop() }
func (timer *systemTimer) Reset(delay time.Duration) bool {
	return timer.timer.Reset(delay)
}

func stopTimer(timer Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.Chan():
		default:
		}
	}
}
