package nav

import (
	"context"
	"time"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type codeActionsResponse struct {
	Path    string                `json:"path"`
	Actions []protocol.CodeAction `json:"actions"`
}

func codeActionsHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, rangeArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args rangeArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		doc, err := manager.EnsureOpen(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		entry, _, _ := deps.Store.Wait(ctx, doc.URI, doc.Version, 150*time.Millisecond)
		actions, err := manager.CodeAction(ctx, &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc.URI},
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(max(args.StartLine-1, 0)), Character: uint32(max(args.StartCharacter-1, 0))},
				End:   protocol.Position{Line: uint32(max(args.EndLine-1, 0)), Character: uint32(max(args.EndCharacter-1, 0))},
			},
			Context: protocol.CodeActionContext{Diagnostics: entry.Diagnostics},
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(codeActionsResponse{Path: args.Path, Actions: actions})
	}
}
