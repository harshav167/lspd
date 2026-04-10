package policy

import (
	"context"
	"sort"
	"sync"

	"github.com/harsha/lspd/internal/config"
	internalformat "github.com/harsha/lspd/internal/format"
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
	filtered := e.filter(diagnostics)
	codeActions := map[string][]string{}
	if e.cfg.AttachCodeActions && e.fetcher != nil {
		for _, diagnostic := range filtered {
			fingerprint := internalformat.Fingerprint(path, diagnostic)
			actions, err := e.fetcher(ctx, path, diagnostic)
			if err == nil && len(actions) > 0 {
				if len(actions) > e.cfg.MaxCodeActionsPerDiagnostic {
					actions = actions[:e.cfg.MaxCodeActionsPerDiagnostic]
				}
				codeActions[fingerprint] = actions
			}
		}
	}
	if sessionID == "" {
		return Result{Diagnostics: filtered, CodeActions: codeActions}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	sessionSet := e.seen[sessionID]
	if sessionSet == nil {
		sessionSet = map[string]struct{}{}
		e.seen[sessionID] = sessionSet
	}
	deduped := make([]protocol.Diagnostic, 0, len(filtered))
	dedupedActions := map[string][]string{}
	for _, diagnostic := range filtered {
		fingerprint := internalformat.Fingerprint(path, diagnostic)
		if _, ok := sessionSet[fingerprint]; ok {
			continue
		}
		sessionSet[fingerprint] = struct{}{}
		deduped = append(deduped, diagnostic)
		if actions, ok := codeActions[fingerprint]; ok {
			dedupedActions[fingerprint] = actions
		}
	}
	return Result{Diagnostics: deduped, CodeActions: dedupedActions}
}

func (e *Engine) filter(diagnostics []protocol.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if !allowSeverity(e.cfg.MinimumSeverity, diagnostic) || !allowSource(e.cfg, diagnostic.Source) {
			continue
		}
		out = append(out, diagnostic)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].Message < out[j].Message
	})
	if len(out) > e.cfg.MaxPerFile {
		out = out[:e.cfg.MaxPerFile]
	}
	if len(out) > e.cfg.MaxPerTurn {
		out = out[:e.cfg.MaxPerTurn]
	}
	return out
}
