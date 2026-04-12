package compat

import (
	"context"

	"github.com/harsha/lspd/internal/metrics"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
)

func stubHandler(name string, metricsRegistry *metrics.Registry) func(context.Context, sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(context.Context, sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if metricsRegistry != nil {
			metricsRegistry.RecordRequest("mcp", name)
		}
		return sdkmcp.NewToolResultText("ok"), nil
	}
}
