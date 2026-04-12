package policy

import (
	"context"

	internalformat "github.com/harsha/lspd/internal/format"
	"go.lsp.dev/protocol"
)

func (e *Engine) codeActionsFor(ctx context.Context, path string, diagnostics []protocol.Diagnostic) map[string][]string {
	if !e.cfg.AttachCodeActions || e.fetcher == nil || len(diagnostics) == 0 {
		return map[string][]string{}
	}
	codeActions := make(map[string][]string, len(diagnostics))
	for _, diagnostic := range diagnostics {
		actions, err := e.fetcher(ctx, path, diagnostic)
		if err != nil || len(actions) == 0 {
			continue
		}
		if e.cfg.MaxCodeActionsPerDiagnostic > 0 && len(actions) > e.cfg.MaxCodeActionsPerDiagnostic {
			actions = actions[:e.cfg.MaxCodeActionsPerDiagnostic]
		}
		codeActions[internalformat.Fingerprint(path, diagnostic)] = actions
	}
	return codeActions
}

// FetchCodeActions wraps the configured fetcher.
func (e *Engine) FetchCodeActions(ctx context.Context, path string, diagnostic protocol.Diagnostic) ([]string, error) {
	if e.fetcher == nil {
		return nil, nil
	}
	return e.fetcher(ctx, path, diagnostic)
}
