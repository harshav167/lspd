package supervisor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/store"
	"github.com/harsha/lspd/internal/metrics"
)

// State represents current supervisor health.
type State string

const (
	StateHealthy    State = "healthy"
	StateRestarting State = "restarting"
	StateDegraded   State = "degraded"
)

// Supervisor restarts failed language servers with backoff.
type Supervisor struct {
	cfg      config.LanguageConfig
	root     string
	store    *store.Store
	logger   *slog.Logger
	metrics  *metrics.Registry
	mu       sync.RWMutex
	state    State
	lastErr  error
	restarts []time.Time
}

// New creates a supervisor.
func New(cfg config.LanguageConfig, root string, diagnosticStore *store.Store, logger *slog.Logger, metricsRegistry *metrics.Registry) *Supervisor {
	return &Supervisor{cfg: cfg, root: root, store: diagnosticStore, logger: logger, metrics: metricsRegistry}
}

// Run waits on the manager and restarts it when it fails.
func (s *Supervisor) Run(ctx context.Context, manager *client.Manager, onReplace func(*client.Manager)) (*client.Manager, error) {
	current := manager
	for {
		err := current.Wait()
		if ctx.Err() != nil {
			return current, ctx.Err()
		}
		s.recordFailure(err)
		replacement, replacementErr := s.restartUntilHealthy(ctx, current, onReplace)
		if replacementErr != nil {
			return current, replacementErr
		}
		current = replacement
	}
}

// State returns the current supervisor state.
func (s *Supervisor) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Supervisor) recordFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
	if s.metrics != nil {
		s.metrics.RecordRestart(s.cfg.Name)
	}
	s.restarts = append(s.restarts, time.Now())
	cutoff := time.Now().Add(-s.cfg.RestartWindow.Duration)
	filtered := s.restarts[:0]
	for _, restart := range s.restarts {
		if restart.After(cutoff) {
			filtered = append(filtered, restart)
		}
	}
	s.restarts = filtered
}

func (s *Supervisor) restartCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.restarts)
}

func (s *Supervisor) tooManyRestarts() bool {
	return s.restartCount() >= s.cfg.MaxRestarts
}

func (s *Supervisor) setState(state State, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.lastErr = err
}

func (s *Supervisor) clearFailures() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = nil
	s.restarts = nil
}

func (s *Supervisor) lastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastErr
}

func (s *Supervisor) restartBackoff() time.Duration {
	restarts := s.restartCount()
	if restarts <= 1 {
		return time.Second
	}
	backoff := time.Second << (restarts - 1)
	if backoff > 8*time.Second {
		return 8 * time.Second
	}
	return backoff
}

func (s *Supervisor) waitBackoff(ctx context.Context, backoff time.Duration) error {
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Supervisor) replaceManager(ctx context.Context, current *client.Manager, onReplace func(*client.Manager)) (*client.Manager, error) {
	replacement, err := client.NewManager(ctx, s.cfg, s.root, s.store, s.logger, s.metrics)
	if err != nil {
		return nil, err
	}
	for _, doc := range current.TrackedDocs() {
		if _, openErr := replacement.EnsureOpen(ctx, doc.Path); openErr != nil {
			s.logger.Debug("document re-registration failed", "language", s.cfg.Name, "path", doc.Path, "error", openErr)
		}
	}
	if onReplace != nil {
		onReplace(replacement)
	}
	return replacement, nil
}

func (s *Supervisor) restartUntilHealthy(ctx context.Context, current *client.Manager, onReplace func(*client.Manager)) (*client.Manager, error) {
	for {
		if s.tooManyRestarts() {
			s.setState(StateDegraded, s.lastError())
			replacement, err := s.probeUntilHealthy(ctx, current, onReplace)
			if err != nil {
				return nil, err
			}
			return replacement, nil
		}
		s.setState(StateRestarting, s.lastError())
		if err := s.waitBackoff(ctx, s.restartBackoff()); err != nil {
			return nil, err
		}
		replacement, err := s.replaceManager(ctx, current, onReplace)
		if err != nil {
			s.recordFailure(err)
			s.setState(StateRestarting, err)
			continue
		}
		s.setState(StateHealthy, nil)
		return replacement, nil
	}
}

func (s *Supervisor) probeUntilHealthy(ctx context.Context, current *client.Manager, onReplace func(*client.Manager)) (*client.Manager, error) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		default:
		}
		replacement, err := s.replaceManager(ctx, current, onReplace)
		if err == nil {
			s.setState(StateHealthy, nil)
			s.clearFailures()
			return replacement, nil
		}
		s.recordFailure(err)
		s.setState(StateDegraded, err)
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		case <-ticker.C:
		}
	}
}
