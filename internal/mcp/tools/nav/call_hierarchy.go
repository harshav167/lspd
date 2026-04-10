package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type callHierarchyArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Direction string `json:"direction"`
}

type callHierarchyResponse struct {
	Item     []protocol.CallHierarchyItem         `json:"item"`
	Incoming []protocol.CallHierarchyIncomingCall `json:"incoming,omitempty"`
	Outgoing []protocol.CallHierarchyOutgoingCall `json:"outgoing,omitempty"`
}

func callHierarchyHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, callHierarchyArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args callHierarchyArgs) (*sdkmcp.CallToolResult, error) {
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		items, err := manager.PrepareCallHierarchy(ctx, &protocol.CallHierarchyPrepareParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + args.Path)},
			Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
		}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		response := callHierarchyResponse{Item: items}
		if len(items) == 0 {
			return responseJSON(response)
		}
		if args.Direction == "incoming" {
			incoming, incomingErr := manager.IncomingCalls(ctx, &protocol.CallHierarchyIncomingCallsParams{Item: items[0]})
			if incomingErr != nil {
				return sdkmcp.NewToolResultError(incomingErr.Error()), nil
			}
			response.Incoming = incoming
		} else {
			outgoing, outgoingErr := manager.OutgoingCalls(ctx, &protocol.CallHierarchyOutgoingCallsParams{Item: items[0]})
			if outgoingErr != nil {
				return sdkmcp.NewToolResultError(outgoingErr.Error()), nil
			}
			response.Outgoing = outgoing
		}
		return responseJSON(response)
	}
}
