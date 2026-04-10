package format

import (
	"fmt"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
)

// SystemReminder formats diagnostics as a Droid system reminder.
func SystemReminder(path string, diagnostics []protocol.Diagnostic, codeActions map[string][]string) string {
	if len(diagnostics) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<system-reminder>\n")
	b.WriteString(fmt.Sprintf("Current diagnostics for %s:\n", filepath.Base(path)))
	errors := 0
	warnings := 0
	for _, diagnostic := range diagnostics {
		switch int(diagnostic.Severity) {
		case 1:
			if errors == 0 {
				b.WriteString("Errors:\n")
			}
			errors++
		case 2:
			if warnings == 0 {
				if errors > 0 {
					b.WriteString("\n")
				}
				b.WriteString("Warnings:\n")
			}
			warnings++
		default:
			continue
		}
		source := ""
		if diagnostic.Source != "" {
			source = fmt.Sprintf(" (%s)", diagnostic.Source)
		}
		b.WriteString(fmt.Sprintf("  - Line %d: %s%s\n", diagnostic.Range.Start.Line+1, diagnostic.Message, source))
		fingerprint := Fingerprint(path, diagnostic)
		if actions := codeActions[fingerprint]; len(actions) > 0 {
			for _, action := range actions {
				b.WriteString(fmt.Sprintf("    quick-fix: %s\n", action))
			}
		}
	}
	b.WriteString("</system-reminder>")
	return b.String()
}
