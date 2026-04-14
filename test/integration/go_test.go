//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"slices"
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

	_, doc, _, err := instance.ResolveDocument(ctx, path)
	if err != nil {
		t.Fatalf("ResolveDocument: %v", err)
	}
	firstEntry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 5*time.Second)
	if waitErr != nil && !ok {
		t.Fatalf("Wait(first): %v", waitErr)
	}
	if !ok || len(firstEntry.Diagnostics) == 0 {
		t.Fatal("expected initial diagnostics")
	}
	states := instance.States()
	if len(states) != 1 {
		t.Fatalf("expected one router state after first resolve, got %#v", states)
	}
	if !slices.Contains(states[0].Documents, path) {
		t.Fatalf("expected router state to track %s, got %#v", path, states[0].Documents)
	}
	if states[0].Supervisor == "" {
		t.Fatalf("expected supervisor state to be populated, got %#v", states[0])
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
	states = instance.States()
	if len(states) != 1 {
		t.Fatalf("expected one router state after restart, got %#v", states)
	}
	if !slices.Contains(states[0].Documents, path) {
		t.Fatalf("expected restarted router state to keep %s open, got %#v", path, states[0].Documents)
	}

	requireEventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		return strings.Count(string(mustReadFile(t, recordFile)), protocol.MethodTextDocumentDidOpen) >= 2
	}, "expected document re-registration after restart")
}

func TestRouterResolveDocumentGuaranteesOpenAndChange(t *testing.T) {
	root := t.TempDir()
	recordFile := filepath.Join(root, "methods.log")
	path := writeFile(t, root, "main.mock", "broken\n")

	cfg := mockRouterConfig(t, mocklsp.ServeOptions{
		RecordFile: recordFile,
		PublishOnOpen: []protocol.Diagnostic{{
			Message:  "opened through router boundary",
			Severity: protocol.DiagnosticSeverityError,
		}},
		PublishOnChange: []protocol.Diagnostic{{
			Message:  "changed through router boundary",
			Severity: protocol.DiagnosticSeverityError,
		}},
	}, false)
	instance, diagnosticStore, ctx := startRouter(t, cfg)

	manager, doc, _, err := instance.ResolveDocument(ctx, path)
	if err != nil {
		t.Fatalf("ResolveDocument(first): %v", err)
	}
	firstEntry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 5*time.Second)
	if waitErr != nil || !ok {
		t.Fatalf("Wait(first): ok=%v err=%v", ok, waitErr)
	}
	if got := firstEntry.Diagnostics[0].Message; got != "opened through router boundary" {
		t.Fatalf("expected open diagnostic, got %q", got)
	}

	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}
	manager2, doc2, _, err := instance.ResolveDocument(ctx, path)
	if err != nil {
		t.Fatalf("ResolveDocument(second): %v", err)
	}
	if manager != manager2 {
		t.Fatal("expected router session to reuse manager for the same workspace")
	}
	secondEntry, ok, waitErr := diagnosticStore.Wait(ctx, doc2.URI, doc2.Version, 5*time.Second)
	if waitErr != nil || !ok {
		t.Fatalf("Wait(second): ok=%v err=%v", ok, waitErr)
	}
	if got := secondEntry.Diagnostics[0].Message; got != "changed through router boundary" {
		t.Fatalf("expected change diagnostic, got %q", got)
	}
	states := instance.States()
	if len(states) != 1 {
		t.Fatalf("expected one router state, got %#v", states)
	}
	if !slices.Contains(states[0].Documents, path) {
		t.Fatalf("expected router state to keep %s open, got %#v", path, states[0].Documents)
	}

	methods := string(mustReadFile(t, recordFile))
	if strings.Count(methods, protocol.MethodTextDocumentDidOpen) != 1 {
		t.Fatalf("expected one didOpen, got log %q", methods)
	}
	if strings.Count(methods, protocol.MethodTextDocumentDidChange) != 1 {
		t.Fatalf("expected one didChange, got log %q", methods)
	}
}
