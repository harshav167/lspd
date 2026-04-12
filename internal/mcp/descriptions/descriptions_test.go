package descriptions

import "testing"

func TestAllDescriptionsAreNonEmpty(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"GetIdeDiagnostics": GetIdeDiagnostics,
		"OpenDiff":          OpenDiff,
		"CloseDiff":         CloseDiff,
		"OpenFile":          OpenFile,
		"LspDefinition":     LspDefinition,
		"LspReferences":     LspReferences,
		"LspHover":          LspHover,
		"LspWorkspaceSymbol": LspWorkspaceSymbol,
		"LspDocumentSymbol": LspDocumentSymbol,
		"LspCodeActions":    LspCodeActions,
		"LspRename":         LspRename,
		"LspFormat":         LspFormat,
		"LspCallHierarchy":  LspCallHierarchy,
		"LspTypeHierarchy":  LspTypeHierarchy,
	}

	for name, value := range values {
		if value == "" {
			t.Fatalf("%s should not be empty", name)
		}
	}
}
