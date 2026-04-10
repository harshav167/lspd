package compat

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	internalformat "github.com/harsha/lspd/internal/format"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	"github.com/harsha/lspd/internal/mcp/descriptions"
	"github.com/harsha/lspd/internal/policy"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	sdkserver "github.com/mark3labs/mcp-go/server"
)

type diagnosticsArgs struct {
	URI string `json:"uri"`
}

type diagnosticsResponse struct {
	Diagnostics []internalformat.IdeDiagnostic `json:"diagnostics"`
}

// Dependencies are required by compatibility handlers.
type Dependencies struct {
	Router        *router.Router
	Store         *store.Store
	Policy        *policy.Engine
	SessionIDFrom func(context.Context) string
}

// Register registers tier-1 compatibility tools.
func Register(server *sdkserver.MCPServer, deps Dependencies) {
	server.AddTool(sdkmcp.NewTool("getIdeDiagnostics",
		sdkmcp.WithDescription(descriptions.GetIdeDiagnostics),
		sdkmcp.WithString("uri", sdkmcp.Required()),
	), sdkmcp.NewTypedToolHandler(func(ctx context.Context, _ sdkmcp.CallToolRequest, args diagnosticsArgs) (*sdkmcp.CallToolResult, error) {
		path, err := pathFromURI(args.URI)
		if err != nil {
			return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
		}
		manager, _, err := deps.Router.Resolve(ctx, path)
		if err != nil {
			return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
		}
		doc, err := manager.EnsureOpen(ctx, path)
		if err != nil {
			return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
		}
		entry, ok, _ := deps.Store.Wait(ctx, doc.URI, doc.Version, 1200*time.Millisecond)
		if !ok {
			return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
		}
		filtered := deps.Policy.Apply(ctx, deps.SessionIDFrom(ctx), path, entry.Diagnostics)
		return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: internalformat.ToIdeDiagnostics(filtered.Diagnostics)})
	}))
	server.AddTool(sdkmcp.NewTool("openDiff", sdkmcp.WithDescription(descriptions.OpenDiff)), stubHandler())
	server.AddTool(sdkmcp.NewTool("closeDiff", sdkmcp.WithDescription(descriptions.CloseDiff)), stubHandler())
	server.AddTool(sdkmcp.NewTool("openFile", sdkmcp.WithDescription(descriptions.OpenFile)), stubHandler())
}

func pathFromURI(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Scheme == "file" {
		return filepath.Clean(parsed.Path), nil
	}
	return "", url.InvalidHostError(raw)
}
