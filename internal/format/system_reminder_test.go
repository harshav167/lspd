package format

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestSystemReminderIncludesQuickFixes(t *testing.T) {
	t.Parallel()
	path := "/tmp/example.go"
	diagnostic := protocol.Diagnostic{
		Message:  "undefined: missingName",
		Severity: protocol.DiagnosticSeverityError,
		Source:   "compiler",
		Range:    protocol.Range{Start: protocol.Position{Line: 9, Character: 13}},
	}
	fingerprint := Fingerprint(path, diagnostic)
	reminder := SystemReminder(path, []protocol.Diagnostic{diagnostic}, map[string][]string{
		fingerprint: {"Create symbol"},
	})
	if !strings.Contains(reminder, "Line 10: undefined: missingName (compiler)") {
		t.Fatalf("missing diagnostic line: %s", reminder)
	}
	if !strings.Contains(reminder, "quick-fix: Create symbol") {
		t.Fatalf("missing quick-fix line: %s", reminder)
	}
}
