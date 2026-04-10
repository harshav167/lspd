//go:build integration

package integration

import (
	"testing"

	"github.com/harsha/lspd/internal/config"
	"go.lsp.dev/protocol"
)

func TestNavigationContracts(t *testing.T) {
	requireBinary(t, "gopls")
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/nav\n\ngo 1.22\n")
	writeFile(t, root, "lib/lib.go", "package lib\n\nfunc Add(a int, b int) int { return a + b }\n")
	path := writeFile(t, root, "main.go", "package main\n\nimport \"example.com/nav/lib\"\n\nfunc main() {\n\t_ = lib.Add(1, 2)\n}\n")
	cfg := config.Default().Languages["go"]
	manager, _, ctx := startManager(t, cfg, root)
	if _, err := manager.EnsureOpen(ctx, path); err != nil {
		t.Fatalf("EnsureOpen: %v", err)
	}
	defs, err := manager.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + path)},
			Position:     protocol.Position{Line: 5, Character: 9},
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected definition result")
	}
	refs, err := manager.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: defs[0].URI},
			Position:     defs[0].Range.Start,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected references result")
	}
}
