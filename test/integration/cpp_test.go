//go:build integration

package integration

import (
	"testing"

	"github.com/harshav167/lspd/internal/config"
)

func TestClangdDiagnostics(t *testing.T) {
	requireBinary(t, "clangd")
	root := t.TempDir()
	writeFile(t, root, "compile_commands.json", "[{\"directory\":\""+root+"\",\"command\":\"clang++ -c main.cpp\",\"file\":\"main.cpp\"}]")
	path := writeFile(t, root, "main.cpp", "#include <string>\nint main() {\n  return missing_symbol;\n}\n")
	cfg := config.Default().Languages["cpp"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	firstVersion, firstEntry := openDocumentAndWait(t, manager, diagnosticStore, ctx, path, false)
	if firstEntry.Version < firstVersion {
		t.Fatalf("entry version %d < client version %d", firstEntry.Version, firstVersion)
	}
	secondVersion, secondEntry := updateDocumentAndWait(t, manager, diagnosticStore, ctx, path, "#include <string>\nint main() {\n  return other_missing_symbol + still_missing_symbol;\n}\n", false)
	if secondVersion <= firstVersion {
		t.Fatalf("expected version to increase, got %d then %d", firstVersion, secondVersion)
	}
	if secondEntry.Version < secondVersion {
		t.Fatalf("entry version %d < client version %d", secondEntry.Version, secondVersion)
	}
}
