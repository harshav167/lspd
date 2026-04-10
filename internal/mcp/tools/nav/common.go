package nav

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/harsha/lspd/internal/format"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	"github.com/harsha/lspd/internal/policy"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

// Dependencies are shared across navigation handlers.
type Dependencies struct {
	Router *router.Router
	Store  *store.Store
	Policy *policy.Engine
}

type positionArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

type rangeArgs struct {
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	StartCharacter int    `json:"start_character"`
	EndLine        int    `json:"end_line"`
	EndCharacter   int    `json:"end_character"`
}

type typeHierarchyItem = client.TypeHierarchyItem

func resolvePosition(ctx context.Context, deps Dependencies, args positionArgs) (Dependencies, protocol.TextDocumentPositionParams, error) {
	manager, _, err := deps.Router.Resolve(ctx, args.Path)
	if err != nil {
		return deps, protocol.TextDocumentPositionParams{}, err
	}
	doc, err := manager.EnsureOpen(ctx, args.Path)
	if err != nil {
		return deps, protocol.TextDocumentPositionParams{}, err
	}
	_ = doc
	return deps, protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + filepath.ToSlash(args.Path))},
		Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
	}, nil
}

func responseJSON[T any](payload T) (*sdkmcp.CallToolResult, error) {
	return sdkmcp.NewToolResultJSON(payload)
}

func pathFromURI(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return filepath.Clean(parsed.Path)
}

func locationFromProtocol(location protocol.Location) format.Location {
	path := pathFromURI(string(location.URI))
	return format.Location{
		Path:         path,
		Line:         int(location.Range.Start.Line) + 1,
		Character:    int(location.Range.Start.Character) + 1,
		EndLine:      int(location.Range.End.Line) + 1,
		EndCharacter: int(location.Range.End.Character) + 1,
		Preview:      format.PreviewLine(path, int(location.Range.Start.Line)),
	}
}

func markdownToText(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case protocol.MarkupContent:
		return typed.Value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
