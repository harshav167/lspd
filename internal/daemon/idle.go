package daemon

import "time"

type idleTimer struct {
	timeout time.Duration
	timer   *time.Timer
}

func newIdleTimer(timeout time.Duration) *idleTimer {
	if timeout <= 0 {
		return &idleTimer{timeout: timeout}
	}
	return &idleTimer{timeout: timeout, timer: time.NewTimer(timeout)}
}

func (i *idleTimer) touch() {
	if i == nil || i.timer == nil {
		return
	}
	if !i.timer.Stop() {
		select {
		case <-i.timer.C:
		default:
		}
	}
	i.timer.Reset(i.timeout)
}
