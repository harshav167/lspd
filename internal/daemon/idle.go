package daemon

import (
	"sync"
	"time"
)

type idleTimer struct {
	timeout time.Duration
	timer   *time.Timer
	mu      sync.Mutex
	last    time.Time
}

func newIdleTimer(timeout time.Duration) *idleTimer {
	if timeout <= 0 {
		return &idleTimer{timeout: timeout, last: time.Now()}
	}
	return &idleTimer{timeout: timeout, timer: time.NewTimer(timeout), last: time.Now()}
}

func (i *idleTimer) touch() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.last = time.Now()
	defer i.mu.Unlock()
	if i.timer == nil {
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

func (i *idleTimer) update(timeout time.Duration) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timeout = timeout
	if timeout <= 0 {
		if i.timer != nil {
			if !i.timer.Stop() {
				select {
				case <-i.timer.C:
				default:
				}
			}
		}
		i.timer = nil
		return
	}
	if i.timer == nil {
		i.timer = time.NewTimer(timeout)
		return
	}
	if !i.timer.Stop() {
		select {
		case <-i.timer.C:
		default:
		}
	}
	i.timer.Reset(timeout)
}

func (i *idleTimer) idleFor(now time.Time) time.Duration {
	if i == nil {
		return 0
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.last.IsZero() {
		return 0
	}
	return now.Sub(i.last)
}
