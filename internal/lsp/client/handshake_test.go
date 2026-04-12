package client

import (
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"go.lsp.dev/protocol"
)

func TestPathToURIUsesFileScheme(t *testing.T) {
	t.Parallel()

	uri := pathToURI("/tmp/example.go")
	if uri != protocol.DocumentURI("file:///tmp/example.go") {
		t.Fatalf("unexpected uri: %q", uri)
	}
}

func TestWithTimeoutUsesManagerRequestTimeout(t *testing.T) {
	t.Parallel()

	manager := &Manager{requestTimeout: 25 * time.Millisecond}
	ctx, cancel := manager.withTimeout(t.Context())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	if time.Until(deadline) > 100*time.Millisecond {
		t.Fatalf("expected short timeout, got %s", time.Until(deadline))
	}
}

func TestEncodeRangeReturnsJSON(t *testing.T) {
	t.Parallel()

	got := encodeRange(protocol.Range{
		Start: protocol.Position{Line: 1, Character: 2},
		End:   protocol.Position{Line: 3, Character: 4},
	})
	if got == "" || got[0] != '{' {
		t.Fatalf("expected json payload, got %q", got)
	}
}

func TestManagerStringIncludesLanguageAndRoot(t *testing.T) {
	t.Parallel()

	manager := &Manager{cfg: config.LanguageConfig{Name: "go"}, root: "/tmp/project"}
	if got := manager.String(); got != "manager{go:/tmp/project}" {
		t.Fatalf("unexpected manager string: %q", got)
	}
}
