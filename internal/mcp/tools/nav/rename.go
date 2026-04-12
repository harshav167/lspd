package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type renameArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	NewName   string `json:"new_name"`
	DryRun    bool   `json:"dry_run"`
}

type renameResponse struct {
	OldName      string    `json:"old_name"`
	NewName      string    `json:"new_name"`
	DryRun       bool      `json:"dry_run"`
	FilesTouched int       `json:"files_touched"`
	TotalEdits   int       `json:"total_edits"`
	Edit         *editPlan `json:"edit,omitempty"`
}

func renameHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, renameArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args renameArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspRename")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		edit, err := manager.Rename(ctx, &protocol.RenameParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)},
				Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
			},
			NewName: args.NewName,
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		oldName := identifierAtPosition(args.Path, args.Line, args.Character)
		editPlan, filesTouched, totalEdits := workspaceEditSummary(edit)
		return responseJSON(renameResponse{
			OldName:      oldName,
			NewName:      args.NewName,
			DryRun:       args.DryRun,
			FilesTouched: filesTouched,
			TotalEdits:   totalEdits,
			Edit:         editPlan,
		})
	}
}
