package mcp

import "context"

type sessionKey struct{}

// WithSessionID stores the MCP session ID in context.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey{}, sessionID)
}

// SessionIDFromContext returns the MCP session ID from context.
func SessionIDFromContext(ctx context.Context) string {
	if sessionID, ok := ctx.Value(sessionKey{}).(string); ok {
		return sessionID
	}
	return ""
}
