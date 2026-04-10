//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
)

func TestClangdDiagnostics(t *testing.T) {
	requireBinary(t, "clangd")
	root := t.TempDir()
	writeFile(t, root, "compile_commands.json", "[{\"directory\":\""+root+"\",\"command\":\"clang++ -c main.cpp\",\"file\":\"main.cpp\"}]")
	path := writeFile(t, root, "main.cpp", "#include <string>\nint main() {\n  return missing_symbol;\n}\n")
	cfg := config.Default().Languages["cpp"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		t.Fatalf("EnsureOpen: %v", err)
	}
	entry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 10*time.Second)
	if waitErr != nil && !ok {
		t.Fatalf("Wait: %v", waitErr)
	}
	if len(entry.Diagnostics) == 0 {
		t.Fatal("expected clangd diagnostics")
	}
}
