package nav

import (
	"context"
	"os"

	"github.com/harshav167/lspd/internal/format"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
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
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		edits, err := manager.Formatting(ctx, &protocol.DocumentFormattingParams{TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)}})
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
