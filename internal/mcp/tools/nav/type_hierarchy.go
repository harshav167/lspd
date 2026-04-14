package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type typeHierarchyArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Direction string `json:"direction"`
}

type typeHierarchyResponse struct {
	Item      *typeHierarchySummary  `json:"item,omitempty"`
	Direction string                 `json:"direction"`
	Types     []typeHierarchySummary `json:"types"`
}

type typeHierarchySummary struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

func typeHierarchyHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, typeHierarchyArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args typeHierarchyArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspTypeHierarchy")
		if args.Direction != "super" && args.Direction != "sub" {
			return sdkmcp.NewToolResultError("direction must be either \"super\" or \"sub\""), nil
		}
		service, err := resolvePositionService(ctx, deps, positionArgs{
			Path:      args.Path,
			Line:      args.Line,
			Character: args.Character,
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		items, err := service.manager.PrepareTypeHierarchy(ctx, service.typeHierarchyPrepareParams())
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		response := typeHierarchyResponse{Direction: args.Direction, Types: []typeHierarchySummary{}}
		if len(items) == 0 {
			return responseJSON(response)
		}
		item := summarizeTypeHierarchyItem(items[0])
		response.Item = &item
		if args.Direction == "super" {
			items, err = service.manager.Supertypes(ctx, items[0])
		} else {
			items, err = service.manager.Subtypes(ctx, items[0])
		}
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		summaries := make([]typeHierarchySummary, 0, len(items))
		for _, item := range items {
			summaries = append(summaries, summarizeTypeHierarchyItem(item))
		}
		response.Types = summaries
		return responseJSON(response)
	}
}

func summarizeTypeHierarchyItem(item typeHierarchyItem) typeHierarchySummary {
	return typeHierarchySummary{
		Name:   item.Name,
		Kind:   symbolKindName(protocol.SymbolKind(item.Kind)),
		Detail: item.Detail,
		Path:   pathFromURI(item.URI),
		Line:   int(item.Range.Start.Line) + 1,
		Column: int(item.Range.Start.Character) + 1,
	}
}
