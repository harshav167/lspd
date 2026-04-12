//go:build integration

package integration

import (
	"testing"

	"github.com/harsha/lspd/internal/config"
)

func TestRustAnalyzerDiagnostics(t *testing.T) {
	requireBinary(t, "rust-analyzer")
	root := t.TempDir()
	writeFile(t, root, "Cargo.toml", "[package]\nname = \"lspd-rust-test\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	path := writeFile(t, root, "src/main.rs", "fn broken() -> String {\n    123\n}\n\nfn main() {\n    println!(\"{}\", missing_name);\n}\n")
	cfg := config.Default().Languages["rust"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	firstVersion, firstEntry := openDocumentAndWait(t, manager, diagnosticStore, ctx, path, true)
	if firstEntry.Version < firstVersion {
		t.Fatalf("entry version %d < client version %d", firstEntry.Version, firstVersion)
	}
	secondVersion, secondEntry := updateDocumentAndWait(t, manager, diagnosticStore, ctx, path, "fn broken() -> String {\n    String::from(123)\n}\n\nfn main() {\n    println!(\"{}\", other_missing_name);\n}\n", true)
	if secondVersion <= firstVersion {
		t.Fatalf("expected version to increase, got %d then %d", firstVersion, secondVersion)
	}
	if secondEntry.Version < secondVersion {
		t.Fatalf("entry version %d < client version %d", secondEntry.Version, secondVersion)
	}
}
