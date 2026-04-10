package compat

import (
	"context"

	sdkmcp "github.com/mark3labs/mcp-go/mcp"
)

func stubHandler() func(context.Context, sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	return func(context.Context, sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return sdkmcp.NewToolResultText("ok"), nil
	}
}
