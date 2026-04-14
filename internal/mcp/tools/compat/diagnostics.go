package compat

import (
	"context"

	internalformat "github.com/harshav167/lspd/internal/format"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/mcp/descriptions"
	"github.com/harshav167/lspd/internal/metrics"
	"github.com/harshav167/lspd/internal/policy"
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
		service := policy.NewDiagnosticsService(deps.Router, deps.Store, deps.Policy)
		result, err := service.Fetch(ctx, policy.DiagnosticsRequest{
			URI:          args.URI,
			SessionID:    deps.SessionIDFrom(ctx),
			Freshness:    policy.DiagnosticsFreshnessBestEffortNow,
			Presentation: policy.DiagnosticsPresentationRaw,
		})
		if err != nil || !result.Found {
			return emptyDiagnosticsResult()
		}
		return diagnosticsResult(result.Entry)
	}))
	server.AddTool(sdkmcp.NewTool("openDiff", sdkmcp.WithDescription(descriptions.OpenDiff)), stubHandler("openDiff", deps.Metrics))
	server.AddTool(sdkmcp.NewTool("closeDiff", sdkmcp.WithDescription(descriptions.CloseDiff)), stubHandler("closeDiff", deps.Metrics))
	server.AddTool(sdkmcp.NewTool("openFile", sdkmcp.WithDescription(descriptions.OpenFile)), stubHandler("openFile", deps.Metrics))
}

func diagnosticsResult(entry store.Entry) (*sdkmcp.CallToolResult, error) {
	return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: internalformat.ToIdeDiagnostics(entry.Diagnostics)})
}

func emptyDiagnosticsResult() (*sdkmcp.CallToolResult, error) {
	return sdkmcp.NewToolResultJSON(diagnosticsResponse{Diagnostics: []internalformat.IdeDiagnostic{}})
}
