//go:build integration

package integration

import (
	"testing"

	"github.com/harshav167/lspd/internal/config"
)

func TestPyrightDiagnostics(t *testing.T) {
	requireBinary(t, "pyright-langserver")
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", "[project]\nname = 'lspd-py-test'\nversion = '0.0.1'\n")
	path := writeFile(t, root, "main.py", "def broken() -> str:\n    return 123\n\nprint(missing_name)\n")
	cfg := config.Default().Languages["py"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	firstVersion, firstEntry := openDocumentAndWait(t, manager, diagnosticStore, ctx, path, true)
	if firstEntry.Version < firstVersion {
		t.Fatalf("entry version %d < client version %d", firstEntry.Version, firstVersion)
	}
	secondVersion, secondEntry := updateDocumentAndWait(t, manager, diagnosticStore, ctx, path, "def broken() -> str:\n    return 456\n\nprint(other_missing)\n", true)
	if secondVersion <= firstVersion {
		t.Fatalf("expected version to increase, got %d then %d", firstVersion, secondVersion)
	}
	if secondEntry.Version < secondVersion {
		t.Fatalf("entry version %d < client version %d", secondEntry.Version, secondVersion)
	}
}
