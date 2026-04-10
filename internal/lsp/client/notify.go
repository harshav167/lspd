package client

import (
	"context"
	"encoding/json"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (m *Manager) handleIncoming(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	switch req.Method() {
	case protocol.MethodTextDocumentPublishDiagnostics:
		var params protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(req.Params(), &params); err == nil {
			m.store.Publish(params.URI, int32(params.Version), params.Diagnostics, m.cfg.Name)
		}
	case protocol.MethodWindowLogMessage, protocol.MethodWindowShowMessage:
		var payload map[string]any
		if err := json.Unmarshal(req.Params(), &payload); err == nil {
			m.logger.Debug("lsp message", "language", m.cfg.Name, "method", req.Method(), "payload", payload)
		}
	default:
		if len(req.Params()) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(req.Params(), &payload); err == nil {
				m.logger.Debug("lsp notification", "language", m.cfg.Name, "method", req.Method(), "payload", payload)
			}
		}
	}
	return reply(ctx, nil, nil)
}
