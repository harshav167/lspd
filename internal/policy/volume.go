package policy

import "go.lsp.dev/protocol"

func allowSeverity(minimum int, diagnostic protocol.Diagnostic) bool {
	if diagnostic.Severity == 0 {
		return true
	}
	return int(diagnostic.Severity) <= minimum+1
}
