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
	Item      *callHierarchyItemSummary `json:"item,omitempty"`
	Direction string                    `json:"direction"`
	Calls     []callHierarchyEdge       `json:"calls,omitempty"`
}

type callHierarchyItemSummary struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type callHierarchyEdge struct {
	From      *callHierarchyItemSummary `json:"from,omitempty"`
	To        *callHierarchyItemSummary `json:"to,omitempty"`
	CallSites []callHierarchySite       `json:"call_sites"`
}

type callHierarchySite struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

func callHierarchyHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, callHierarchyArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args callHierarchyArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspCallHierarchy")
		if args.Direction != "incoming" && args.Direction != "outgoing" {
			return sdkmcp.NewToolResultError("direction must be either \"incoming\" or \"outgoing\""), nil
		}
		service, err := resolvePositionService(ctx, deps, positionArgs{
			Path:      args.Path,
			Line:      args.Line,
			Character: args.Character,
		})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		items, err := service.manager.PrepareCallHierarchy(ctx, service.callHierarchyPrepareParams())
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		response := callHierarchyResponse{Direction: args.Direction}
		if len(items) == 0 {
			return responseJSON(response)
		}
		item := summarizeCallHierarchyItem(items[0])
		response.Item = &item
		if args.Direction == "incoming" {
			incoming, incomingErr := service.manager.IncomingCalls(ctx, &protocol.CallHierarchyIncomingCallsParams{Item: items[0]})
			if incomingErr != nil {
				return sdkmcp.NewToolResultError(incomingErr.Error()), nil
			}
			response.Calls = summarizeIncomingCalls(incoming)
		} else {
			outgoing, outgoingErr := service.manager.OutgoingCalls(ctx, &protocol.CallHierarchyOutgoingCallsParams{Item: items[0]})
			if outgoingErr != nil {
				return sdkmcp.NewToolResultError(outgoingErr.Error()), nil
			}
			response.Calls = summarizeOutgoingCalls(items[0], outgoing)
		}
		return responseJSON(response)
	}
}

func summarizeCallHierarchyItem(item protocol.CallHierarchyItem) callHierarchyItemSummary {
	return callHierarchyItemSummary{
		Name:   item.Name,
		Kind:   symbolKindName(item.Kind),
		Detail: item.Detail,
		Path:   pathFromURI(string(item.URI)),
		Line:   int(item.Range.Start.Line) + 1,
		Column: int(item.Range.Start.Character) + 1,
	}
}

func summarizeIncomingCalls(calls []protocol.CallHierarchyIncomingCall) []callHierarchyEdge {
	out := make([]callHierarchyEdge, 0, len(calls))
	for _, call := range calls {
		edge := callHierarchyEdge{
			From: pointerToCallHierarchyItemSummary(summarizeCallHierarchyItem(call.From)),
		}
		for _, site := range call.FromRanges {
			edge.CallSites = append(edge.CallSites, callHierarchySite{
				Path:   pathFromURI(string(call.From.URI)),
				Line:   int(site.Start.Line) + 1,
				Column: int(site.Start.Character) + 1,
			})
		}
		out = append(out, edge)
	}
	return out
}

func summarizeOutgoingCalls(source protocol.CallHierarchyItem, calls []protocol.CallHierarchyOutgoingCall) []callHierarchyEdge {
	out := make([]callHierarchyEdge, 0, len(calls))
	for _, call := range calls {
		edge := callHierarchyEdge{
			To: pointerToCallHierarchyItemSummary(summarizeCallHierarchyItem(call.To)),
		}
		for _, site := range call.FromRanges {
			edge.CallSites = append(edge.CallSites, callHierarchySite{
				Path:   pathFromURI(string(source.URI)),
				Line:   int(site.Start.Line) + 1,
				Column: int(site.Start.Character) + 1,
			})
		}
		out = append(out, edge)
	}
	return out
}

func pointerToCallHierarchyItemSummary(item callHierarchyItemSummary) *callHierarchyItemSummary {
	return &item
}
