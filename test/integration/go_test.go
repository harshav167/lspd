//go:build integration

package integration

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/test/mocklsp"
	"go.lsp.dev/protocol"
)

func TestGoplsDiagnostics(t *testing.T) {
	requireBinary(t, "gopls")
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/test\n\ngo 1.22\n")
	path := writeFile(t, root, "main.go", "package main\n\nfunc broken() string {\n\treturn 123\n}\n")
	cfg := config.Default().Languages["go"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	firstVersion, firstEntry := openDocumentAndWait(t, manager, diagnosticStore, ctx, path, true)
	if firstEntry.Version < firstVersion {
		t.Fatalf("entry version %d < client version %d", firstEntry.Version, firstVersion)
	}
	secondVersion, secondEntry := updateDocumentAndWait(t, manager, diagnosticStore, ctx, path, "package main\n\nfunc broken() string {\n\tvar value string = 123\n\treturn value\n}\n", true)
	if secondVersion <= firstVersion {
		t.Fatalf("expected version to increase, got %d then %d", firstVersion, secondVersion)
	}
	if secondEntry.Version < secondVersion {
		t.Fatalf("entry version %d < client version %d", secondEntry.Version, secondVersion)
	}
}

func TestSupervisorRestartsAndReregistersDocuments(t *testing.T) {
	root := t.TempDir()
	recordFile := filepath.Join(root, "methods.log")
	crashMarker := filepath.Join(root, "crashed.once")
	path := writeFile(t, root, "main.mock", "broken\n")

	cfg := mockRouterConfig(t, mocklsp.ServeOptions{
		RecordFile:         recordFile,
		CrashMarkerFile:    crashMarker,
		CrashAfterOpenOnce: true,
		PublishOnOpen: []protocol.Diagnostic{{
			Message:  "mock restart diagnostic",
			Severity: protocol.DiagnosticSeverityError,
		}},
	}, false)
	instance, diagnosticStore, ctx := startRouter(t, cfg)

	manager, _, err := instance.Resolve(ctx, path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		t.Fatalf("EnsureOpen: %v", err)
	}
	firstEntry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 5*time.Second)
	if waitErr != nil && !ok {
		t.Fatalf("Wait(first): %v", waitErr)
	}
	if !ok || len(firstEntry.Diagnostics) == 0 {
		t.Fatal("expected initial diagnostics")
	}

	secondEntry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, firstEntry.Version+1, 10*time.Second)
	if waitErr != nil && !ok {
		t.Fatalf("Wait(second): %v", waitErr)
	}
	if !ok || len(secondEntry.Diagnostics) == 0 {
		t.Fatal("expected diagnostics after restart")
	}
	if secondEntry.Version <= firstEntry.Version {
		t.Fatalf("expected restart publish to advance version, got %d then %d", firstEntry.Version, secondEntry.Version)
	}

	requireEventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		return strings.Count(string(mustReadFile(t, recordFile)), protocol.MethodTextDocumentDidOpen) >= 2
	}, "expected document re-registration after restart")
}
