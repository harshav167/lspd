//go:build integration

package integration

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/test/mocklsp"
	"go.lsp.dev/protocol"
)

func TestTypeScriptDiagnostics(t *testing.T) {
	requireBinary(t, "typescript-language-server")
	root := t.TempDir()
	writeFile(t, root, "package.json", "{\n  \"name\": \"lspd-ts-test\"\n}\n")
	writeFile(t, root, "tsconfig.json", "{\n  \"compilerOptions\": {\n    \"strict\": true,\n    \"noEmit\": true\n  }\n}\n")
	path := writeFile(t, root, "index.ts", "const value: string =\nconsole.log(missingName)\n")
	cfg := config.Default().Languages["ts"]
	manager, diagnosticStore, ctx := startManager(t, cfg, root)
	firstVersion, firstEntry := openDocumentAndWait(t, manager, diagnosticStore, ctx, path, true)
	if firstEntry.Version < firstVersion {
		t.Fatalf("entry version %d < client version %d", firstEntry.Version, firstVersion)
	}
	secondVersion, secondEntry := updateDocumentAndWait(t, manager, diagnosticStore, ctx, path, "const value: number = 'oops'\nconsole.log(otherMissing)\n", true)
	if secondVersion <= firstVersion {
		t.Fatalf("expected version to increase, got %d then %d", firstVersion, secondVersion)
	}
	if secondEntry.Version < secondVersion {
		t.Fatalf("entry version %d < client version %d", secondEntry.Version, secondVersion)
	}
}

func TestManagerWarmupIssuesWorkspaceSymbolRequest(t *testing.T) {
	root := t.TempDir()
	recordFile := filepath.Join(root, "methods.log")
	path := writeFile(t, root, "index.mock", "broken\n")
	options := mocklsp.ServeOptions{
		RecordFile:    recordFile,
		PublishOnOpen: []protocol.Diagnostic{{Message: "mock diagnostic", Severity: protocol.DiagnosticSeverityError}},
	}
	manager, diagnosticStore, ctx := startManager(t, mockLanguageConfig(t, options, true), root)
	_, _ = openDocumentAndWait(t, manager, diagnosticStore, ctx, path, false)
	requireEventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		return fileContains(recordFile, protocol.MethodWorkspaceSymbol)
	}, "expected warmup workspace/symbol request")
}
