package policy

import (
	"context"

	"go.lsp.dev/protocol"
)

// FetchCodeActions wraps the configured fetcher.
func (e *Engine) FetchCodeActions(ctx context.Context, path string, diagnostic protocol.Diagnostic) ([]string, error) {
	if e.fetcher == nil {
		return nil, nil
	}
	return e.fetcher(ctx, path, diagnostic)
}
