package nav

import (
	"context"
	"fmt"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type workspaceSymbolArgs struct {
	Query string `json:"query"`
	Path  string `json:"path,omitempty"`
}

type workspaceSymbolResponse struct {
	Query   string                       `json:"query"`
	Symbols []protocol.SymbolInformation `json:"symbols"`
}

func workspaceSymbolHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, workspaceSymbolArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args workspaceSymbolArgs) (*sdkmcp.CallToolResult, error) {
		manager, err := managerForPath(ctx, deps, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		symbols, err := manager.WorkspaceSymbol(ctx, &protocol.WorkspaceSymbolParams{Query: args.Query})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(workspaceSymbolResponse{Query: args.Query, Symbols: symbols})
	}
}

func managerForPath(ctx context.Context, deps Dependencies, path string) (interface {
	WorkspaceSymbol(context.Context, *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error)
}, error) {
	if path != "" {
		manager, _, err := deps.Router.Resolve(ctx, path)
		return manager, err
	}
	for _, manager := range deps.Router.Snapshot() {
		return manager, nil
	}
	return nil, fmt.Errorf("path is required before any language server has been started")
}
