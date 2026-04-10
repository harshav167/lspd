package socket

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/harsha/lspd/internal/lsp/store"
	"go.lsp.dev/protocol"
)

func TestServerPing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lspd.sock")
	diagnosticStore := store.New()
	server := NewServer(path, diagnosticStore, Callbacks{
		Peek:   func(context.Context, Request) (store.Entry, bool, error) { return store.Entry{}, false, nil },
		Drain:  func(context.Context, Request) (store.Entry, bool, error) { return store.Entry{}, false, nil },
		Forget: func(Request) {},
		Reload: func(context.Context) error { return nil },
		Status: func() map[string]any { return map[string]any{"port": 1234} },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()
	diagnosticStore.Publish(protocol.DocumentURI("/tmp/example.ts"), 1, nil, "ts")
	time.Sleep(20 * time.Millisecond)

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Request{Op: "ping"}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !response.OK || response.Message != "pong" {
		t.Fatalf("unexpected response: %+v", response)
	}
}
