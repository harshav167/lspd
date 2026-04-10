package supervisor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/store"
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
	mu       sync.RWMutex
	state    State
	lastErr  error
	restarts []time.Time
}

// New creates a supervisor.
func New(cfg config.LanguageConfig, root string, diagnosticStore *store.Store, logger *slog.Logger) *Supervisor {
	return &Supervisor{cfg: cfg, root: root, store: diagnosticStore, logger: logger, state: StateHealthy}
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
		if s.tooManyRestarts() {
			s.setState(StateDegraded, err)
			replacement, probeErr := s.probeUntilHealthy(ctx, current, onReplace)
			if probeErr != nil {
				return current, probeErr
			}
			current = replacement
			continue
		}
		restarts := s.restartCount()
		s.setState(StateRestarting, err)
		backoff := time.Duration(restarts) * time.Second
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return current, ctx.Err()
		case <-timer.C:
		}
		replacement, replacementErr := client.NewManager(ctx, s.cfg, s.root, s.store, s.logger)
		if replacementErr != nil {
			return current, replacementErr
		}
		for _, doc := range current.TrackedDocs() {
			if _, openErr := replacement.EnsureOpen(ctx, doc.Path); openErr != nil {
				s.logger.Debug("document re-registration failed", "language", s.cfg.Name, "path", doc.Path, "error", openErr)
			}
		}
		if onReplace != nil {
			onReplace(replacement)
		}
		s.setState(StateHealthy, nil)
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

func (s *Supervisor) probeUntilHealthy(ctx context.Context, current *client.Manager, onReplace func(*client.Manager)) (*client.Manager, error) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		default:
		}
		replacement, err := client.NewManager(ctx, s.cfg, s.root, s.store, s.logger)
		if err == nil {
			for _, doc := range current.TrackedDocs() {
				if _, openErr := replacement.EnsureOpen(ctx, doc.Path); openErr != nil {
					s.logger.Debug("document re-registration failed", "language", s.cfg.Name, "path", doc.Path, "error", openErr)
				}
			}
			if onReplace != nil {
				onReplace(replacement)
			}
			s.setState(StateHealthy, nil)
			s.mu.Lock()
			s.restarts = nil
			s.mu.Unlock()
			return replacement, nil
		}
		s.setState(StateDegraded, err)
		select {
		case <-ctx.Done():
			return current, ctx.Err()
		case <-ticker.C:
		}
	}
}
