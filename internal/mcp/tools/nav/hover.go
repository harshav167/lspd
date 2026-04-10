package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type hoverResponse struct {
	Contents string          `json:"contents"`
	Range    *protocol.Range `json:"range,omitempty"`
}

func hoverHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, positionArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args positionArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		hover, err := manager.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + args.Path)},
			Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
		}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if hover == nil {
			return responseJSON(hoverResponse{})
		}
		return responseJSON(hoverResponse{Contents: markdownToText(hover.Contents), Range: hover.Range})
	}
}
