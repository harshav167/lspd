package store

import (
	"context"
	"testing"
	"time"

	"go.lsp.dev/protocol"
)

func TestStoreWait(t *testing.T) {
	t.Parallel()
	store := New()
	uri := protocol.DocumentURI("file:///tmp/example.ts")

	go func() {
		time.Sleep(20 * time.Millisecond)
		store.Publish(uri, 2, []protocol.Diagnostic{{Message: "broken", Severity: protocol.DiagnosticSeverityError}}, "ts")
	}()

	entry, ok, err := store.Wait(context.Background(), uri, 2, time.Second)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected entry")
	}
	if entry.Version != 2 {
		t.Fatalf("got version %d", entry.Version)
	}
}
