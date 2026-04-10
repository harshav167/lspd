package socket

import (
	"context"
	"time"

	"github.com/harsha/lspd/internal/lsp/store"
	"go.lsp.dev/protocol"
)

type handler struct {
	store       *store.Store
	policyReset func(string)
	reload      func(context.Context) error
	status      func() map[string]any
}

func (h *handler) handle(ctx context.Context, request Request) Response {
	switch request.Op {
	case "ping":
		return Response{OK: true, Message: "pong", Time: time.Now()}
	case "reload":
		if h.reload != nil {
			if err := h.reload(ctx); err != nil {
				return Response{Message: err.Error()}
			}
		}
		return Response{OK: true, Message: "reloaded", Time: time.Now()}
	case "forget":
		h.store.Forget(protocol.DocumentURI(request.Path))
		return Response{OK: true, Message: "forgotten"}
	case "peek", "drain":
		entry, ok := h.store.Peek(protocol.DocumentURI(request.Path))
		if !ok {
			return Response{OK: true, Message: "not_found"}
		}
		if request.Op == "drain" && h.policyReset != nil {
			h.policyReset(request.SessionID)
		}
		return Response{OK: true, Entry: &entry}
	case "status":
		response := Response{OK: true, Entries: h.store.Snapshot(), Time: time.Now()}
		if h.status != nil {
			response.Status = h.status()
		}
		return response
	default:
		return Response{Message: "unknown operation"}
	}
}
