package compat

import (
	"context"
	"testing"

	"github.com/harshav167/lspd/internal/metrics"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
)

func TestStubHandlerReturnsOKAndRecordsMetrics(t *testing.T) {
	t.Parallel()

	registry := metrics.New()
	handler := stubHandler("openDiff", registry)

	result, err := handler(context.Background(), sdkmcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("expected successful tool result, got %#v", result)
	}
}
