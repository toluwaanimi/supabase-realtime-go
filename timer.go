package supabase_realtime_go

import (
	"time"
)

type Timer struct {
	timer     *time.Timer
	tries     int
	callback  func()
	timerCalc func(tries int) time.Duration
}

func NewTimer(timerCalc func(tries int) time.Duration, callback func()) *Timer {
	return &Timer{
		tries:     0,
		callback:  callback,
		timerCalc: timerCalc,
	}
}

func (t *Timer) Reset() {
	t.tries = 0
	if t.timer != nil {
		t.timer.Stop()
	}
}

func (t *Timer) ScheduleTimeout() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(t.timerCalc(t.tries+1), func() {
		t.tries++
		t.callback()
	})
}
