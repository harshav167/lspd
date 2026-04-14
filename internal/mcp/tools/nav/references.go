package nav

import (
	"context"
	"sort"

	"github.com/harshav167/lspd/internal/format"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
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
		service, err := resolvePositionService(ctx, deps, positionArgs{
			Path:      args.Path,
			Line:      args.Line,
			Character: args.Character,
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		locations, err := service.manager.References(ctx, service.referenceParams(args.IncludeDeclaration))
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
