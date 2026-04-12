package nav

import (
	"context"
	"sort"

	"github.com/harshav167/lspd/internal/format"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type referencesArgs struct {
	Path               string `json:"path"`
	Line               int    `json:"line"`
	Character          int    `json:"character"`
	IncludeDeclaration bool   `json:"include_declaration"`
}

type referencesByFile struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type referencesResponse struct {
	Total      int                `json:"total"`
	Truncated  bool               `json:"truncated,omitempty"`
	Omitted    int                `json:"omitted,omitempty"`
	References []format.Location  `json:"references"`
	ByFile     []referencesByFile `json:"by_file"`
}

func referencesHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, referencesArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args referencesArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspReferences")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		locations, err := manager.References(ctx, &protocol.ReferenceParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)},
				Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
			},
			Context: protocol.ReferenceContext{IncludeDeclaration: args.IncludeDeclaration},
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		byFile := map[string]int{}
		response := referencesResponse{Total: len(locations), References: make([]format.Location, 0, len(locations))}
		for _, location := range locations {
			converted := locationFromProtocol(location)
			byFile[converted.Path]++
			if len(response.References) < 100 {
				response.References = append(response.References, converted)
			}
		}
		sortLocations(response.References)
		response.Truncated = len(locations) > len(response.References)
		response.Omitted = len(locations) - len(response.References)
		for path, count := range byFile {
			response.ByFile = append(response.ByFile, referencesByFile{Path: path, Count: count})
		}
		sort.SliceStable(response.ByFile, func(i, j int) bool {
			if response.ByFile[i].Count != response.ByFile[j].Count {
				return response.ByFile[i].Count > response.ByFile[j].Count
			}
			return response.ByFile[i].Path < response.ByFile[j].Path
		})
		return responseJSON(response)
	}
}
