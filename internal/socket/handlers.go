package socket

import (
	"context"
	"time"

	"github.com/harshav167/lspd/internal/lsp/store"
)

type handler struct {
	store         *store.Store
	peek          func(context.Context, Request) (store.Entry, map[string][]string, bool, error)
	drain         func(context.Context, Request) (store.Entry, map[string][]string, bool, error)
	forget        func(Request)
	reload        func(context.Context) error
	status        func() map[string]any
	touch         func()
	recordRequest func(surface, method string)
}

func (h *handler) handle(ctx context.Context, request Request) Response {
	if h.touch != nil {
		h.touch()
	}
	if h.recordRequest != nil {
		h.recordRequest("socket", request.Op)
	}
	callCtx := ctx
	cancel := func() {}
	if request.TimeoutMs > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(request.TimeoutMs)*time.Millisecond)
	}
	defer cancel()
	switch request.Op {
	case "ping":
		return Response{OK: true, Message: "pong", Time: time.Now()}
	case "reload":
		if h.reload != nil {
			if err := h.reload(callCtx); err != nil {
				return Response{Message: err.Error()}
			}
		}
		return Response{OK: true, Message: "reloaded", Time: time.Now()}
	case "forget":
		if h.forget != nil {
			h.forget(request)
		}
		return Response{OK: true, Message: "forgotten"}
	case "peek", "drain":
		var (
			entry       store.Entry
			codeActions map[string][]string
			ok          bool
			err         error
		)
		if request.Op == "drain" && request.Kind == "read" {
			if h.peek != nil {
				entry, codeActions, ok, err = h.peek(callCtx, request)
			}
			if err == nil && !ok && h.drain != nil {
				entry, codeActions, ok, err = h.drain(callCtx, request)
			}
		} else if request.Op == "drain" && h.drain != nil {
			entry, codeActions, ok, err = h.drain(callCtx, request)
		} else if h.peek != nil {
			entry, codeActions, ok, err = h.peek(callCtx, request)
		}
		if err != nil {
			return Response{Message: err.Error()}
		}
		if !ok {
			return Response{OK: true, Message: "not_found"}
		}
		return Response{OK: true, Entry: &entry, CodeActions: codeActions}
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
