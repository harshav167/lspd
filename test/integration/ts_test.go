//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
)

func TestTypeScriptDiagnostics(t *testing.T) {
	requireBinary(t, "typescript-language-server")
	root := t.TempDir()
	writeFile(t, root, "package.json", "{\n  \"name\": \"lspd-ts-test\"\n}\n")
	writeFile(t, root, "tsconfig.json", "{\n  \"compilerOptions\": {\n    \"strict\": true,\n    \"noEmit\": true\n  }\n}\n")
	path := writeFile(t, root, "index.ts", "const value: string =\nconsole.log(missingName)\n")
	cfg := config.Default().Languages["ts"]
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
		t.Fatal("expected typescript diagnostics")
	}
}
