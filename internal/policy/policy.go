package policy

import (
	"context"
	"sync"

	"github.com/harshav167/lspd/internal/config"
	"go.lsp.dev/protocol"
)

// CodeActionFetcher fetches code action titles for a diagnostic.
type CodeActionFetcher func(context.Context, string, protocol.Diagnostic) ([]string, error)

// Engine applies diagnostic surfacing rules.
type Engine struct {
	mu      sync.Mutex
	seen    map[string]map[string]struct{}
	cfg     config.PolicyConfig
	fetcher CodeActionFetcher
}

// Result is the filtered diagnostics plus quick-fix previews.
type Result struct {
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
	CodeActions map[string][]string   `json:"code_actions,omitempty"`
}

// New creates a new policy engine.
func New(cfg config.PolicyConfig, fetcher CodeActionFetcher) *Engine {
	return &Engine{seen: map[string]map[string]struct{}{}, cfg: cfg, fetcher: fetcher}
}

// UpdateConfig updates runtime config.
func (e *Engine) UpdateConfig(cfg config.PolicyConfig) { e.cfg = cfg }

// Apply applies policy filters and remembers emitted diagnostics per session.
func (e *Engine) Apply(ctx context.Context, sessionID string, path string, diagnostics []protocol.Diagnostic) Result {
	filtered := filterDiagnostics(e.cfg, diagnostics)
	filtered = e.dedup(sessionID, path, filtered)
	filtered = limitDiagnostics(filtered, e.cfg.MaxPerFile, e.cfg.MaxPerTurn)
	return Result{
		Diagnostics: filtered,
		CodeActions: e.codeActionsFor(ctx, path, filtered),
	}
}
