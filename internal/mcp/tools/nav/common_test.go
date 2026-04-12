package nav

import (
	"testing"

	"go.lsp.dev/protocol"
)

func TestUTF16HelpersHandleEmoji(t *testing.T) {
	t.Parallel()

	value := "a😀b"
	if got := utf16Len(value); got != 4 {
		t.Fatalf("expected utf16 length 4, got %d", got)
	}
	if got := safeSliceUTF16(value, 0, 3); got != "a😀" {
		t.Fatalf("unexpected slice: %q", got)
	}
}

func TestApplyTextEditsUsesUTF16Offsets(t *testing.T) {
	t.Parallel()

	content := "a😀b\n"
	edits := []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 3},
			End:   protocol.Position{Line: 0, Character: 4},
		},
		NewText: "c",
	}}

	if got := applyTextEdits(content, edits); got != "a😀c\n" {
		t.Fatalf("unexpected formatted text: %q", got)
	}
}

func TestWorkspaceEditSummaryCountsFilesAndChanges(t *testing.T) {
	t.Parallel()

	edit := &protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			protocol.DocumentURI("file:///tmp/a.go"): {{
				Range:   protocol.Range{},
				NewText: "a",
			}},
			protocol.DocumentURI("file:///tmp/b.go"): {{
				Range:   protocol.Range{},
				NewText: "b",
			}},
		},
	}

	plan, files, changes := workspaceEditSummary(edit)
	if plan == nil || files != 2 || changes != 2 {
		t.Fatalf("unexpected summary: plan=%#v files=%d changes=%d", plan, files, changes)
	}
}
