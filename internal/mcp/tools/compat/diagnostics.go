package compat

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	internalformat "github.com/harshav167/lspd/internal/format"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/mcp/descriptions"
	"github.com/harshav167/lspd/internal/metrics"
	"github.com/harshav167/lspd/internal/policy"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	sdkserver "github.com/mark3labs/mcp-go/server"
	"go.lsp.dev/protocol"
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
	Metrics       *metrics.Registry
}

// Register registers tier-1 compatibility tools.
func Register(server *sdkserver.MCPServer, deps Dependencies) {
	server.AddTool(sdkmcp.NewTool("getIdeDiagnostics",
		sdkmcp.WithDescription(descriptions.GetIdeDiagnostics),
		sdkmcp.WithString("uri", sdkmcp.Required()),
	), sdkmcp.NewTypedToolHandler(func(ctx context.Context, _ sdkmcp.CallToolRequest, args diagnosticsArgs) (*sdkmcp.CallToolResult, error) {
		if deps.Metrics != nil {
			deps.Metrics.RecordRequest("mcp", "getIdeDiagnostics")
		}
		path, err := pathFromURI(args.URI)
		if err != nil {
			return emptyDiagnosticsResult()
		}
		uri := protocol.DocumentURI(args.URI)
		cached, cachedOK := deps.Store.Peek(uri)
		if deps.Router == nil {
			return diagnosticsResult(ctx, deps, path, cached, cachedOK)
		}
		manager, _, err := deps.Router.Resolve(ctx, path)
		if err != nil {
			return diagnosticsResult(ctx, deps, path, cached, cachedOK)
		}
		doc, err := manager.EnsureOpen(ctx, path)
		if err != nil {
			return diagnosticsResult(ctx, deps, path, cached, cachedOK)
		}
		entry, ok, _ := deps.Store.Wait(ctx, doc.URI, doc.Version, 1200*time.Millisecond)
		if ok && (entry.PublishedVersion > 0 || entry.Version >= doc.Version) {
			return diagnosticsResult(ctx, deps, path, entry, true)
		}
		return diagnosticsResult(ctx, deps, path, cached, cachedOK)
	}))
	server.AddTool(sdkmcp.NewTool("openDiff", sdkmcp.WithDescription(descriptions.OpenDiff)), stubHandler("openDiff", deps.Metrics))
	server.AddTool(sdkmcp.NewTool("closeDiff", sdkmcp.WithDescription(descriptions.CloseDiff)), stubHandler("closeDiff", deps.Metrics))
	server.AddTool(sdkmcp.NewTool("openFile", sdkmcp.WithDescription(descriptions.OpenFile)), stubHandler("openFile", deps.Metrics))
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

func diagnosticsResult(ctx context.Context, deps Dependencies, path string, entry store.Entry, ok bool) (*sdkmcp.CallToolResult, error) {
	if !ok {
		return emptyDiagnosticsResult()
	}
	// Don't run session dedup here. Droid's own compareDiagnostics(before, after)
	// handles the "only show new errors" diffing for write-time injection.
	// If we dedup here, the first fetchDiagnostics call (Droid's internal before/after
	// pipeline) marks everything as delivered, and all subsequent calls — including
	// the model's explicit getIdeDiagnostics and the read hook — return empty.
	// Return raw diagnostics and let Droid handle the presentation logic.
	return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: internalformat.ToIdeDiagnostics(entry.Diagnostics)})
}

func emptyDiagnosticsResult() (*sdkmcp.CallToolResult, error) {
	return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
}
