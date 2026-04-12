package mcp

import (
	"context"
	"net/http"

	sdkserver "github.com/mark3labs/mcp-go/server"
)

const mcpSessionHeader = "Mcp-Session-Id"

type sessionKey struct{}

// WithSessionID stores a fallback session ID in context.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey{}, sessionID)
}

// RequestSessionID extracts the real MCP StreamableHTTP session header and falls back
// to any legacy configured header name when present.
func RequestSessionID(r *http.Request, fallbackHeader string) string {
	if r == nil {
		return ""
	}
	if sessionID := r.Header.Get(mcpSessionHeader); sessionID != "" {
		return sessionID
	}
	if fallbackHeader == "" || fallbackHeader == mcpSessionHeader {
		return ""
	}
	return r.Header.Get(fallbackHeader)
}

// SessionIDFromContext returns the MCP session ID from context.
func SessionIDFromContext(ctx context.Context) string {
	if session := sdkserver.ClientSessionFromContext(ctx); session != nil && session.SessionID() != "" {
		return session.SessionID()
	}
	if sessionID, ok := ctx.Value(sessionKey{}).(string); ok {
		return sessionID
	}
	return ""
}
