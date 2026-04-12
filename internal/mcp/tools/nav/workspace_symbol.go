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
	Query     string                `json:"query"`
	Truncated bool                  `json:"truncated,omitempty"`
	Omitted   int                   `json:"omitted,omitempty"`
	Symbols   []workspaceSymbolItem `json:"symbols"`
}

type workspaceSymbolItem struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Container string `json:"container,omitempty"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
}

func workspaceSymbolHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, workspaceSymbolArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args workspaceSymbolArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspWorkspaceSymbol")
		manager, err := managerForPath(ctx, deps, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		symbols, err := manager.WorkspaceSymbol(ctx, &protocol.WorkspaceSymbolParams{Query: args.Query})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		items := make([]workspaceSymbolItem, 0, len(symbols))
		for _, symbol := range symbols {
			if len(items) == 100 {
				break
			}
			items = append(items, workspaceSymbolItem{
				Name:      symbol.Name,
				Kind:      symbolKindName(symbol.Kind),
				Container: symbol.ContainerName,
				Path:      pathFromURI(string(symbol.Location.URI)),
				Line:      int(symbol.Location.Range.Start.Line) + 1,
				Column:    int(symbol.Location.Range.Start.Character) + 1,
			})
		}
		return responseJSON(workspaceSymbolResponse{
			Query:     args.Query,
			Truncated: len(symbols) > len(items),
			Omitted:   len(symbols) - len(items),
			Symbols:   items,
		})
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
