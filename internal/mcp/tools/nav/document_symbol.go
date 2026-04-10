package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type documentSymbolArgs struct {
	Path string `json:"path"`
}

type documentSymbolResponse struct {
	Path    string        `json:"path"`
	Symbols []interface{} `json:"symbols"`
}

func documentSymbolHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, documentSymbolArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args documentSymbolArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		symbols, err := manager.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + args.Path)}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(documentSymbolResponse{Path: args.Path, Symbols: symbols})
	}
}
