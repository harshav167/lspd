package socket

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/lsp/store"
)

func TestServerOnExitCalledOnUnexpectedListenerClose(t *testing.T) {
	path := filepath.Join("/tmp", "lspd-socket-exit-test.sock")
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })
	server := NewServer(path, store.New(), Callbacks{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan struct{}, 1)
	server.OnExit(func(err error) {
		done <- struct{}{}
	})
	if err := server.listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected OnExit callback")
	}
}
