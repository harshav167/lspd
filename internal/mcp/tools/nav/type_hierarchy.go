package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
)

type typeHierarchyArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Direction string `json:"direction"`
}

type typeHierarchyResponse struct {
	Items []typeHierarchyItem `json:"items"`
}

func typeHierarchyHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, typeHierarchyArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args typeHierarchyArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		items, err := manager.PrepareTypeHierarchy(ctx, map[string]any{
			"textDocument": map[string]any{"uri": "file://" + args.Path},
			"position": map[string]any{
				"line":      max(args.Line-1, 0),
				"character": max(args.Character-1, 0),
			},
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if len(items) == 0 {
			return responseJSON(typeHierarchyResponse{})
		}
		if args.Direction == "super" {
			items, err = manager.Supertypes(ctx, items[0])
		} else {
			items, err = manager.Subtypes(ctx, items[0])
		}
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(typeHierarchyResponse{Items: items})
	}
}
