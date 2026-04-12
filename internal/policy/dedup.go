package policy

import (
	internalformat "github.com/harshav167/lspd/internal/format"
	"go.lsp.dev/protocol"
)

func (e *Engine) dedup(sessionID string, path string, diagnostics []protocol.Diagnostic) []protocol.Diagnostic {
	if sessionID == "" || len(diagnostics) == 0 {
		return diagnostics
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	sessionSet := e.seen[sessionID]
	if sessionSet == nil {
		sessionSet = map[string]struct{}{}
		e.seen[sessionID] = sessionSet
	}

	deduped := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		fingerprint := internalformat.Fingerprint(path, diagnostic)
		if _, ok := sessionSet[fingerprint]; ok {
			continue
		}
		sessionSet[fingerprint] = struct{}{}
		deduped = append(deduped, diagnostic)
	}
	return deduped
}

// ResetSession clears remembered diagnostics for a session.
func (e *Engine) ResetSession(sessionID string) {
	if sessionID == "" {
		return
	}
	e.mu.Lock()
	delete(e.seen, sessionID)
	e.mu.Unlock()
}
