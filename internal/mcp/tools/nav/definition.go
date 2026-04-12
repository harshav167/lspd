package nav

import (
	"context"

	"github.com/harsha/lspd/internal/format"
	"github.com/harsha/lspd/internal/mcp/descriptions"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	sdkserver "github.com/mark3labs/mcp-go/server"
	"go.lsp.dev/protocol"
)

type definitionsResponse struct {
	Definitions []format.Location `json:"definitions"`
}

// Register registers the semantic navigation tools.
func Register(server *sdkserver.MCPServer, deps Dependencies) {
	server.AddTool(sdkmcp.NewTool("lspDefinition", sdkmcp.WithDescription(descriptions.LspDefinition), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(definitionHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspReferences", sdkmcp.WithDescription(descriptions.LspReferences), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required()), sdkmcp.WithBoolean("include_declaration")), sdkmcp.NewTypedToolHandler(referencesHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspHover", sdkmcp.WithDescription(descriptions.LspHover), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(hoverHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspWorkspaceSymbol", sdkmcp.WithDescription(descriptions.LspWorkspaceSymbol), sdkmcp.WithString("query", sdkmcp.Required()), sdkmcp.WithString("path")), sdkmcp.NewTypedToolHandler(workspaceSymbolHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspDocumentSymbol", sdkmcp.WithDescription(descriptions.LspDocumentSymbol), sdkmcp.WithString("path", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(documentSymbolHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspCodeActions", sdkmcp.WithDescription(descriptions.LspCodeActions), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("start_line", sdkmcp.Required()), sdkmcp.WithNumber("start_character", sdkmcp.Required()), sdkmcp.WithNumber("end_line", sdkmcp.Required()), sdkmcp.WithNumber("end_character", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(codeActionsHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspRename", sdkmcp.WithDescription(descriptions.LspRename), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required()), sdkmcp.WithString("new_name", sdkmcp.Required()), sdkmcp.WithBoolean("dry_run")), sdkmcp.NewTypedToolHandler(renameHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspFormat", sdkmcp.WithDescription(descriptions.LspFormat), sdkmcp.WithString("path", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(formatHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspCallHierarchy", sdkmcp.WithDescription(descriptions.LspCallHierarchy), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required()), sdkmcp.WithString("direction", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(callHierarchyHandler(deps)))
	server.AddTool(sdkmcp.NewTool("lspTypeHierarchy", sdkmcp.WithDescription(descriptions.LspTypeHierarchy), sdkmcp.WithString("path", sdkmcp.Required()), sdkmcp.WithNumber("line", sdkmcp.Required()), sdkmcp.WithNumber("character", sdkmcp.Required()), sdkmcp.WithString("direction", sdkmcp.Required())), sdkmcp.NewTypedToolHandler(typeHierarchyHandler(deps)))
}

func definitionHandler(deps Dependencies) func(context.Context, sdkmcp.CallToolRequest, positionArgs) (*sdkmcp.CallToolResult, error) {
	return func(ctx context.Context, _ sdkmcp.CallToolRequest, args positionArgs) (*sdkmcp.CallToolResult, error) {
		recordToolRequest(deps, "lspDefinition")
		manager, _, err := deps.Router.Resolve(ctx, args.Path)
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		if _, err := manager.EnsureOpen(ctx, args.Path); err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		locations, err := manager.Definition(ctx, &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: documentURI(args.Path)},
			Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
		}})
		if err != nil {
			return sdkmcp.NewToolResultError(err.Error()), nil
		}
		response := definitionsResponse{Definitions: make([]format.Location, 0, len(locations))}
		for _, location := range locations {
			response.Definitions = append(response.Definitions, locationFromProtocol(location))
		}
		sortLocations(response.Definitions)
		return responseJSON(response)
	}
}
