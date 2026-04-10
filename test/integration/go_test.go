//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
)

func TestGoplsDiagnostics(t *testing.T) {
	requireBinary(t, "gopls")
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/test\n\ngo 1.22\n")
	path := writeFile(t, root, "main.go", "package main\n\nfunc broken() string {\n\treturn 123\n}\n")
	cfg := config.Default().Languages["go"]
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
		t.Fatal("expected gopls diagnostics")
	}
}
