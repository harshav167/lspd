package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type formatArgs struct {
	Path string `json:"path"`
}

type formatResponse struct {
	Path  string              `json:"path"`
	Edits []protocol.TextEdit `json:"edits"`
}

func formatHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, formatArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args formatArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		edits, err := manager.Formatting(ctx, &protocol.DocumentFormattingParams{TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + args.Path)}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(formatResponse{Path: args.Path, Edits: edits})
	}
}
