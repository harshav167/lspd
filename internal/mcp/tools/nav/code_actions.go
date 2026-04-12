package nav

import (
	"context"
	"time"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type codeActionsResponse struct {
	Path    string              `json:"path"`
	Actions []codeActionSummary `json:"actions"`
}

type codeActionSummary struct {
	Title          string          `json:"title"`
	Kind           string          `json:"kind,omitempty"`
	IsPreferred    bool            `json:"is_preferred,omitempty"`
	DisabledReason string          `json:"disabled_reason,omitempty"`
	Edit           *editPlan       `json:"edit,omitempty"`
	Command        *commandSummary `json:"command,omitempty"`
}

func codeActionsHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, rangeArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args rangeArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspCodeActions")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		doc, err := manager.EnsureOpen(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		entry, _, _ := deps.Store.Wait(ctx, doc.URI, doc.Version, 500*time.Millisecond)
		actions, err := manager.CodeAction(ctx, &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc.URI},
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(max(args.StartLine-1, 0)), Character: uint32(max(args.StartCharacter-1, 0))},
				End:   protocol.Position{Line: uint32(max(args.EndLine-1, 0)), Character: uint32(max(args.EndCharacter-1, 0))},
			},
			Context: protocol.CodeActionContext{Diagnostics: entry.Diagnostics},
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		summaries := make([]codeActionSummary, 0, len(actions))
		for _, action := range actions {
			summary := codeActionSummary{
				Title:       action.Title,
				Kind:        string(action.Kind),
				IsPreferred: action.IsPreferred,
			}
			if action.Disabled != nil {
				summary.DisabledReason = action.Disabled.Reason
			}
			if edit, _, _ := workspaceEditSummary(action.Edit); edit != nil {
				summary.Edit = edit
			}
			if action.Command != nil {
				summary.Command = &commandSummary{
					Title:     action.Command.Title,
					Command:   action.Command.Command,
					Arguments: action.Command.Arguments,
				}
			}
			summaries = append(summaries, summary)
		}
		return responseJSON(codeActionsResponse{Path: args.Path, Actions: summaries})
	}
}
