package daemon

import "testing"

func TestNewIdleTimerDisablesNonPositiveTimeouts(t *testing.T) {
	t.Parallel()
	if timer := newIdleTimer(0); timer.timer != nil {
		t.Fatal("expected nil timer for zero timeout")
	}
	if timer := newIdleTimer(-1); timer.timer != nil {
		t.Fatal("expected nil timer for negative timeout")
	}
}
