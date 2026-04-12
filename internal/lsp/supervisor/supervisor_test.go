package supervisor

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/store"
)

func testSupervisor() *Supervisor {
	return New(
		config.LanguageConfig{
			Name:          "go",
			MaxRestarts:   5,
			RestartWindow: config.Duration{Duration: time.Minute},
		},
		"/tmp",
		store.New(),
		slog.New(slog.NewTextHandler(nilDiscard{}, nil)),
		nil,
	)
}

type nilDiscard struct{}

func (nilDiscard) Write(p []byte) (int, error) { return len(p), nil }

func TestRecordFailureTracksSlidingWindow(t *testing.T) {
	t.Parallel()

	s := testSupervisor()
	s.recordFailure(errors.New("boom"))
	if got := s.restartCount(); got != 1 {
		t.Fatalf("expected restart count 1, got %d", got)
	}
}

func TestTooManyRestartsHonorsMaxRestarts(t *testing.T) {
	t.Parallel()

	s := testSupervisor()
	s.cfg.MaxRestarts = 1
	s.recordFailure(errors.New("boom"))
	if !s.tooManyRestarts() {
		t.Fatal("expected tooManyRestarts to be true")
	}
}

func TestRestartBackoffCapsAtEightSeconds(t *testing.T) {
	t.Parallel()

	s := testSupervisor()
	s.restarts = []time.Time{time.Now(), time.Now(), time.Now(), time.Now(), time.Now()}
	if got := s.restartBackoff(); got != 8*time.Second {
		t.Fatalf("expected capped backoff, got %s", got)
	}
}

func TestWaitBackoffHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	s := testSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.waitBackoff(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestClearFailuresResetsState(t *testing.T) {
	t.Parallel()

	s := testSupervisor()
	s.recordFailure(errors.New("boom"))
	s.clearFailures()
	if s.restartCount() != 0 {
		t.Fatalf("expected restart history cleared, got %d", s.restartCount())
	}
	if s.lastError() != nil {
		t.Fatalf("expected last error cleared, got %v", s.lastError())
	}
}
