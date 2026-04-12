package mcp

import (
	"context"
	"net/http"
	"testing"
)

func TestRequestSessionIDPrefersMCPHeader(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(mcpSessionHeader, "mcp-session")
	req.Header.Set("X-Droid-Session-Id", "fallback")

	if got := RequestSessionID(req, "X-Droid-Session-Id"); got != "mcp-session" {
		t.Fatalf("expected MCP session header, got %q", got)
	}
}

func TestSessionIDFromContextFallsBackToInjectedValue(t *testing.T) {
	t.Parallel()

	ctx := WithSessionID(context.Background(), "legacy")
	if got := SessionIDFromContext(ctx); got != "legacy" {
		t.Fatalf("expected fallback session id, got %q", got)
	}
}
