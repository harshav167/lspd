package nav

import (
	"testing"

	"go.lsp.dev/protocol"
)

func TestSummarizeIncomingCallsUsesCallerPathForSites(t *testing.T) {
	t.Parallel()

	calls := []protocol.CallHierarchyIncomingCall{{
		From: protocol.CallHierarchyItem{
			Name: "caller",
			Kind: protocol.SymbolKindFunction,
			URI:  protocol.DocumentURI("file:///tmp/caller.go"),
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 2},
			},
		},
		FromRanges: []protocol.Range{{
			Start: protocol.Position{Line: 3, Character: 4},
		}},
	}}

	summary := summarizeIncomingCalls(calls)
	if len(summary) != 1 || len(summary[0].CallSites) != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary[0].CallSites[0].Path != "/tmp/caller.go" {
		t.Fatalf("unexpected call site path: %#v", summary[0].CallSites[0])
	}
}

func TestSummarizeOutgoingCallsUsesSourcePathForSites(t *testing.T) {
	t.Parallel()

	source := protocol.CallHierarchyItem{
		Name: "source",
		Kind: protocol.SymbolKindFunction,
		URI:  protocol.DocumentURI("file:///tmp/source.go"),
	}
	calls := []protocol.CallHierarchyOutgoingCall{{
		To: protocol.CallHierarchyItem{
			Name: "callee",
			Kind: protocol.SymbolKindFunction,
			URI:  protocol.DocumentURI("file:///tmp/callee.go"),
		},
		FromRanges: []protocol.Range{{
			Start: protocol.Position{Line: 5, Character: 6},
		}},
	}}

	summary := summarizeOutgoingCalls(source, calls)
	if len(summary) != 1 || len(summary[0].CallSites) != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary[0].CallSites[0].Path != "/tmp/source.go" {
		t.Fatalf("unexpected call site path: %#v", summary[0].CallSites[0])
	}
}
