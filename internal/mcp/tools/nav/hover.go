package nav

import (
	"context"
	"strings"

	"github.com/harshav167/lspd/internal/format"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

type hoverResponse struct {
	TypeSignature string        `json:"type_signature,omitempty"`
	Documentation string        `json:"documentation,omitempty"`
	Range         *format.Range `json:"range,omitempty"`
}

func hoverHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, positionArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args positionArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspHover")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		hover, err := manager.Hover(ctx, &protocol.HoverParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)},
			Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
		}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if hover == nil {
			return responseJSON(hoverResponse{})
		}
		var responseRange *format.Range
		if hover.Range != nil {
			rangeValue := toFormatRange(*hover.Range)
			responseRange = &rangeValue
		}
		typeSignature, documentation := splitHoverContents(markdownToText(hover.Contents))
		return responseJSON(hoverResponse{TypeSignature: typeSignature, Documentation: documentation, Range: responseRange})
	}
}

func splitHoverContents(raw string) (string, string) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if raw == "" {
		return "", ""
	}
	if strings.HasPrefix(raw, "```") {
		fenceBody := raw[3:]
		if newline := strings.IndexByte(fenceBody, '\n'); newline >= 0 {
			fenceBody = fenceBody[newline+1:]
		}
		if closing := strings.Index(fenceBody, "\n```"); closing >= 0 {
			signature := strings.TrimSpace(fenceBody[:closing])
			documentation := strings.TrimSpace(fenceBody[closing+4:])
			return signature, trimFencedMarkdown(documentation)
		}
	}
	sections := strings.SplitN(raw, "\n\n", 2)
	if len(sections) == 1 {
		return "", trimFencedMarkdown(raw)
	}
	return strings.TrimSpace(trimFencedMarkdown(sections[0])), strings.TrimSpace(trimFencedMarkdown(sections[1]))
}

func trimFencedMarkdown(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, "```") {
		return raw
	}
	var parts []string
	for {
		start := strings.Index(raw, "```")
		if start < 0 {
			if strings.TrimSpace(raw) != "" {
				parts = append(parts, strings.TrimSpace(raw))
			}
			break
		}
		if strings.TrimSpace(raw[:start]) != "" {
			parts = append(parts, strings.TrimSpace(raw[:start]))
		}
		raw = raw[start+3:]
		if newline := strings.IndexByte(raw, '\n'); newline >= 0 {
			raw = raw[newline+1:]
		}
		end := strings.Index(raw, "```")
		if end < 0 {
			if strings.TrimSpace(raw) != "" {
				parts = append(parts, strings.TrimSpace(raw))
			}
			break
		}
		if strings.TrimSpace(raw[:end]) != "" {
			parts = append(parts, strings.TrimSpace(raw[:end]))
		}
		raw = raw[end+3:]
	}
	return strings.Join(parts, "\n\n")
}
