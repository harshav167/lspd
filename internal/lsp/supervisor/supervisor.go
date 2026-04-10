package supervisor

import (
	"context"
	"log/slog"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/store"
)

// Supervisor restarts failed language servers with backoff.
type Supervisor struct {
	cfg    config.LanguageConfig
	root   string
	store  *store.Store
	logger *slog.Logger
}

// New creates a supervisor.
func New(cfg config.LanguageConfig, root string, diagnosticStore *store.Store, logger *slog.Logger) *Supervisor {
	return &Supervisor{cfg: cfg, root: root, store: diagnosticStore, logger: logger}
}

// Run waits on the manager and restarts it when it fails.
func (s *Supervisor) Run(ctx context.Context, manager *client.Manager) (*client.Manager, error) {
	restarts := 0
	current := manager
	for {
		err := current.Wait()
		if ctx.Err() != nil {
			return current, ctx.Err()
		}
		restarts++
		if restarts > s.cfg.MaxRestarts {
			return current, err
		}
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
		current = replacement
	}
}
