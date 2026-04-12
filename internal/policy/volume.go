package policy

import "go.lsp.dev/protocol"

func limitDiagnostics(diagnostics []protocol.Diagnostic, maxPerFile int, maxPerTurn int) []protocol.Diagnostic {
	limit := len(diagnostics)
	if maxPerFile > 0 && maxPerFile < limit {
		limit = maxPerFile
	}
	if maxPerTurn > 0 && maxPerTurn < limit {
		limit = maxPerTurn
	}
	if limit == len(diagnostics) {
		return diagnostics
	}
	return diagnostics[:limit]
}
