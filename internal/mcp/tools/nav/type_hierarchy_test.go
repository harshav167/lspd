package nav

import (
	"testing"

	"go.lsp.dev/protocol"
)

func TestSummarizeTypeHierarchyItemMapsFields(t *testing.T) {
	t.Parallel()

	item := typeHierarchyItem{
		Name:   "Adder",
		Kind:   5,
		Detail: "interface",
		URI:    "file:///tmp/main.go",
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 4},
		},
	}

	summary := summarizeTypeHierarchyItem(item)
	if summary.Name != "Adder" || summary.Path != "/tmp/main.go" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.Line != 3 || summary.Column != 5 {
		t.Fatalf("expected 1-indexed coordinates, got %#v", summary)
	}
}
