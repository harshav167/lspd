package nav

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type documentSymbolArgs struct {
	Path string `json:"path"`
}

type documentSymbolResponse struct {
	Path    string               `json:"path"`
	Symbols []documentSymbolNode `json:"symbols"`
}

type documentSymbolNode struct {
	Name      string               `json:"name"`
	Kind      string               `json:"kind"`
	Detail    string               `json:"detail,omitempty"`
	Path      string               `json:"path"`
	Line      int                  `json:"line"`
	Column    int                  `json:"column"`
	EndLine   int                  `json:"end_line,omitempty"`
	EndColumn int                  `json:"end_column,omitempty"`
	Children  []documentSymbolNode `json:"children,omitempty"`
}

func documentSymbolHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, documentSymbolArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args documentSymbolArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspDocumentSymbol")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		symbols, err := manager.DocumentSymbol(ctx, &protocol.DocumentSymbolParams{TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		return responseJSON(documentSymbolResponse{Path: args.Path, Symbols: convertDocumentSymbols(args.Path, symbols)})
	}
}

func convertDocumentSymbols(path string, symbols []interface{}) []documentSymbolNode {
	out := make([]documentSymbolNode, 0, len(symbols))
	for _, symbol := range symbols {
		if converted, ok := convertDocumentSymbol(path, symbol); ok {
			out = append(out, converted)
		}
	}
	return out
}

func convertDocumentSymbol(path string, symbol interface{}) (documentSymbolNode, bool) {
	var doc protocol.DocumentSymbol
	if decodeInto(symbol, &doc) && doc.Name != "" {
		node := documentSymbolNode{
			Name:      doc.Name,
			Kind:      symbolKindName(doc.Kind),
			Detail:    doc.Detail,
			Path:      path,
			Line:      int(doc.Range.Start.Line) + 1,
			Column:    int(doc.Range.Start.Character) + 1,
			EndLine:   int(doc.Range.End.Line) + 1,
			EndColumn: int(doc.Range.End.Character) + 1,
		}
		for _, child := range doc.Children {
			node.Children = append(node.Children, documentSymbolNode{
				Name:      child.Name,
				Kind:      symbolKindName(child.Kind),
				Detail:    child.Detail,
				Path:      path,
				Line:      int(child.Range.Start.Line) + 1,
				Column:    int(child.Range.Start.Character) + 1,
				EndLine:   int(child.Range.End.Line) + 1,
				EndColumn: int(child.Range.End.Character) + 1,
				Children:  convertDocumentSymbolChildren(path, child.Children),
			})
		}
		return node, true
	}

	var info protocol.SymbolInformation
	if decodeInto(symbol, &info) && info.Name != "" {
		return documentSymbolNode{
			Name:      info.Name,
			Kind:      symbolKindName(info.Kind),
			Path:      pathFromURI(string(info.Location.URI)),
			Line:      int(info.Location.Range.Start.Line) + 1,
			Column:    int(info.Location.Range.Start.Character) + 1,
			EndLine:   int(info.Location.Range.End.Line) + 1,
			EndColumn: int(info.Location.Range.End.Character) + 1,
		}, true
	}

	return documentSymbolNode{}, false
}

func convertDocumentSymbolChildren(path string, children []protocol.DocumentSymbol) []documentSymbolNode {
	out := make([]documentSymbolNode, 0, len(children))
	for _, child := range children {
		out = append(out, documentSymbolNode{
			Name:      child.Name,
			Kind:      symbolKindName(child.Kind),
			Detail:    child.Detail,
			Path:      path,
			Line:      int(child.Range.Start.Line) + 1,
			Column:    int(child.Range.Start.Character) + 1,
			EndLine:   int(child.Range.End.Line) + 1,
			EndColumn: int(child.Range.End.Character) + 1,
			Children:  convertDocumentSymbolChildren(path, child.Children),
		})
	}
	return out
}
