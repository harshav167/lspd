//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
)

func TestPyrightDiagnostics(t *testing.T) {
	requireBinary(t, "pyright-langserver")
	root := t.TempDir()
	writeFile(t, root, "pyproject.toml", "[project]\nname = 'lspd-py-test'\nversion = '0.0.1'\n")
	path := writeFile(t, root, "main.py", "def broken() -> str:\n    return 123\n\nprint(missing_name)\n")
	cfg := config.Default().Languages["py"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		t.Fatalf("EnsureOpen: %v", err)
	}
	if err := manager.Save(ctx, doc.URI, doc.Content); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 10*time.Second)
	if waitErr != nil && !ok {
		t.Fatalf("Wait: %v", waitErr)
	}
	if len(entry.Diagnostics) == 0 {
		t.Fatal("expected pyright diagnostics")
	}
}
