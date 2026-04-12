package socket

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/lsp/store"
	"go.lsp.dev/protocol"
)

func TestServerPing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lspd.sock")
	diagnosticStore := store.New()
	server := NewServer(path, diagnosticStore, Callbacks{
		Peek: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			return store.Entry{}, nil, false, nil
		},
		Drain: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			return store.Entry{}, nil, false, nil
		},
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

func TestServerReadDrainUsesPeek(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lspd.sock")
	diagnosticStore := store.New()
	var peekCalls, drainCalls int
	server := NewServer(path, diagnosticStore, Callbacks{
		Peek: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			peekCalls++
			return store.Entry{URI: protocol.DocumentURI("file:///tmp/example.ts"), Version: 3}, nil, true, nil
		},
		Drain: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			drainCalls++
			return store.Entry{}, nil, false, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Request{Op: "drain", Kind: "read", TimeoutMs: 25}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !response.OK || response.Entry == nil || response.Entry.Version != 3 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if peekCalls != 1 || drainCalls != 0 {
		t.Fatalf("expected peek only, got peek=%d drain=%d", peekCalls, drainCalls)
	}
}

func TestServerReadDrainFallsBackToDrainWhenCacheMissing(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	path := filepath.Join("/tmp", filepath.Base(tempDir)+".sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	diagnosticStore := store.New()
	var peekCalls, drainCalls int
	server := NewServer(path, diagnosticStore, Callbacks{
		Peek: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			peekCalls++
			return store.Entry{}, nil, false, nil
		},
		Drain: func(context.Context, Request) (store.Entry, map[string][]string, bool, error) {
			drainCalls++
			return store.Entry{URI: protocol.DocumentURI("file:///tmp/example.ts"), Version: 4}, map[string][]string{"x": []string{"fix"}}, true, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Request{Op: "drain", Kind: "read", TimeoutMs: 25}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !response.OK || response.Entry == nil || response.Entry.Version != 4 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if peekCalls != 1 || drainCalls != 1 {
		t.Fatalf("expected peek then drain fallback, got peek=%d drain=%d", peekCalls, drainCalls)
	}
	if len(response.CodeActions) != 1 {
		t.Fatalf("expected code actions in response, got %+v", response.CodeActions)
	}
}

func TestServerTimeoutCancelsDrain(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lspd.sock")
	diagnosticStore := store.New()
	server := NewServer(path, diagnosticStore, Callbacks{
		Drain: func(ctx context.Context, _ Request) (store.Entry, map[string][]string, bool, error) {
			<-ctx.Done()
			return store.Entry{}, nil, false, ctx.Err()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.Close()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(Request{Op: "drain", TimeoutMs: 25}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response.OK || response.Message == "" {
		t.Fatalf("expected timeout error, got %+v", response)
	}
}
