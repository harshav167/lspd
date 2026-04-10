//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
)

func TestRustAnalyzerDiagnostics(t *testing.T) {
	requireBinary(t, "rust-analyzer")
	root := t.TempDir()
	writeFile(t, root, "Cargo.toml", "[package]\nname = \"lspd-rust-test\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	path := writeFile(t, root, "src/main.rs", "fn broken() -> String {\n    123\n}\n\nfn main() {\n    println!(\"{}\", missing_name);\n}\n")
	cfg := config.Default().Languages["rust"]
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
		t.Fatal("expected rust diagnostics")
	}
}
