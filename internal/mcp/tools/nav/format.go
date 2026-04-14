package nav

import (
	"context"
	"os"

	"github.com/harshav167/lspd/internal/format"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
)

type formatArgs struct {
	Path string `json:"path"`
}

type formatResponse struct {
	Path    string        `json:"path"`
	Range   *format.Range `json:"range"`
	Changed bool          `json:"changed"`
	NewText string        `json:"new_text"`
}

func formatHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, formatArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args formatArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspFormat")
		service, err := resolveDocumentService(ctx, deps, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		edits, err := service.manager.Formatting(ctx, service.formattingParams())
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		content, readErr := os.ReadFile(args.Path)
		if readErr != nil {
			return sdkmcp.NewToolResultError(readErr.Error()), nil
		}
		newText := applyTextEdits(string(content), edits)
		return responseJSON(formatResponse{Path: args.Path, Range: nil, Changed: newText != string(content), NewText: newText})
	}
}
