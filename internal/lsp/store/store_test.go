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

func TestStoreWaitHonorsMinimumVersionPerWaiter(t *testing.T) {
	t.Parallel()
	store := New()
	uri := protocol.DocumentURI("file:///tmp/example.ts")

	waitV2 := make(chan Entry, 1)
	waitV3 := make(chan Entry, 1)
	go func() {
		entry, _, err := store.Wait(context.Background(), uri, 2, time.Second)
		if err != nil {
			t.Errorf("Wait(version=2): %v", err)
			return
		}
		waitV2 <- entry
	}()
	go func() {
		entry, _, err := store.Wait(context.Background(), uri, 3, time.Second)
		if err != nil {
			t.Errorf("Wait(version=3): %v", err)
			return
		}
		waitV3 <- entry
	}()

	time.Sleep(20 * time.Millisecond)
	store.Publish(uri, 2, []protocol.Diagnostic{{Message: "first"}}, "ts")

	select {
	case entry := <-waitV2:
		if entry.Version != 2 {
			t.Fatalf("version=2 waiter received version %d", entry.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("version=2 waiter did not unblock")
	}

	select {
	case <-waitV3:
		t.Fatal("version=3 waiter unblocked too early")
	case <-time.After(50 * time.Millisecond):
	}

	store.Publish(uri, 3, []protocol.Diagnostic{{Message: "second"}}, "ts")

	select {
	case entry := <-waitV3:
		if entry.Version != 3 {
			t.Fatalf("version=3 waiter received version %d", entry.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("version=3 waiter did not unblock")
	}
}

func TestStorePublishKeepsVersionsMonotonicAcrossRestart(t *testing.T) {
	t.Parallel()
	store := New()
	uri := protocol.DocumentURI("file:///tmp/example.ts")

	store.Publish(uri, 2, []protocol.Diagnostic{{Message: "before restart"}}, "ts")

	go func() {
		time.Sleep(20 * time.Millisecond)
		store.Publish(uri, 1, []protocol.Diagnostic{{Message: "after restart"}}, "ts")
	}()

	entry, ok, err := store.Wait(context.Background(), uri, 3, time.Second)
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected entry")
	}
	if entry.Version != 3 {
		t.Fatalf("got version %d", entry.Version)
	}
	if len(entry.Diagnostics) != 1 || entry.Diagnostics[0].Message != "after restart" {
		t.Fatalf("unexpected diagnostics: %#v", entry.Diagnostics)
	}
}

func TestStoreVersionlessDiagnosticsKeepEffectiveVersionAndMarkPublishedVersion(t *testing.T) {
	t.Parallel()
	store := New()
	uri := protocol.DocumentURI("file:///tmp/example.ts")

	store.Publish(uri, 2, []protocol.Diagnostic{{Message: "before"}}, "ts")

	store.Publish(uri, 0, []protocol.Diagnostic{{Message: "versionless"}}, "ts")

	entry, ok := store.Peek(uri)
	if !ok {
		t.Fatal("expected latest entry")
	}
	if entry.Version != 3 {
		t.Fatalf("expected effective version 3, got %d", entry.Version)
	}
	if entry.PublishedVersion != 0 {
		t.Fatalf("expected published version 0, got %d", entry.PublishedVersion)
	}
	if len(entry.Diagnostics) != 1 || entry.Diagnostics[0].Message != "versionless" {
		t.Fatalf("unexpected diagnostics: %#v", entry.Diagnostics)
	}
}
