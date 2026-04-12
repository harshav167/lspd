package policy

import (
	"context"
	"testing"

	"github.com/harshav167/lspd/internal/config"
	"go.lsp.dev/protocol"
)

func TestPolicyDedupAndCodeActions(t *testing.T) {
	t.Parallel()
	engine := New(config.PolicyConfig{
		MaxPerFile:                  20,
		MaxPerTurn:                  50,
		MinimumSeverity:             1,
		AttachCodeActions:           true,
		MaxCodeActionsPerDiagnostic: 1,
	}, func(context.Context, string, protocol.Diagnostic) ([]string, error) {
		return []string{"Add import"}, nil
	})

	diagnostic := protocol.Diagnostic{
		Message:  "missing import",
		Severity: protocol.DiagnosticSeverityError,
		Range:    protocol.Range{Start: protocol.Position{Line: 4, Character: 2}},
	}

	first := engine.Apply(context.Background(), "s1", "/tmp/example.ts", []protocol.Diagnostic{diagnostic})
	if len(first.Diagnostics) != 1 {
		t.Fatalf("expected first diagnostic, got %d", len(first.Diagnostics))
	}
	second := engine.Apply(context.Background(), "s1", "/tmp/example.ts", []protocol.Diagnostic{diagnostic})
	if len(second.Diagnostics) != 0 {
		t.Fatalf("expected deduped diagnostics, got %d", len(second.Diagnostics))
	}
}
