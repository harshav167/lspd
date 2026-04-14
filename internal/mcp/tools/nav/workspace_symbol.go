package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
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
		service, err := resolveWorkspaceService(ctx, deps, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		symbols, err := service.manager.WorkspaceSymbol(ctx, service.workspaceSymbolParams(args.Query))
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
