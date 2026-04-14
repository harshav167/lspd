package e2e

import (
	"context"
	"testing"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/policy"
	"go.lsp.dev/protocol"
)

func TestDiagnosticsServiceBestEffortRawFallsBackToCachedEntry(t *testing.T) {
	t.Parallel()

	diagnosticStore := store.New()
	service := policy.NewDiagnosticsService(nil, diagnosticStore, policy.New(config.Default().Policy, nil))

	path := t.TempDir() + "/cached.go"
	uri := store.URIFromPath(path)
	diagnosticStore.Publish(uri, 1, []protocol.Diagnostic{{
		Message:  "cached failure",
		Severity: protocol.DiagnosticSeverityError,
	}}, "go")

	result, err := service.Fetch(context.Background(), policy.DiagnosticsRequest{
		Path:         path,
		Freshness:    policy.DiagnosticsFreshnessBestEffortNow,
		Presentation: policy.DiagnosticsPresentationRaw,
		SessionID:    "session-a",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !result.Found {
		t.Fatal("expected cached diagnostics")
	}
	if len(result.Entry.Diagnostics) != 1 || result.Entry.Diagnostics[0].Message != "cached failure" {
		t.Fatalf("unexpected raw diagnostics: %#v", result.Entry.Diagnostics)
	}
}

func TestDiagnosticsServiceSurfacedPresentationDedupsPerSession(t *testing.T) {
	t.Parallel()

	diagnosticStore := store.New()
	engine := policy.New(config.Default().Policy, func(context.Context, string, protocol.Diagnostic) ([]string, error) {
		return []string{"apply fix"}, nil
	})
	service := policy.NewDiagnosticsService(nil, diagnosticStore, engine)

	path := t.TempDir() + "/surfaced.go"
	uri := store.URIFromPath(path)
	diagnosticStore.Publish(uri, 1, []protocol.Diagnostic{{
		Message:  "surface me",
		Severity: protocol.DiagnosticSeverityError,
	}}, "go")

	req := policy.DiagnosticsRequest{
		Path:         path,
		SessionID:    "session-surfaced",
		Freshness:    policy.DiagnosticsFreshnessPeek,
		Presentation: policy.DiagnosticsPresentationSurfaced,
	}

	first, err := service.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch(first): %v", err)
	}
	if !first.Found || len(first.Entry.Diagnostics) != 1 {
		t.Fatalf("expected surfaced diagnostics on first fetch, got %#v", first.Entry.Diagnostics)
	}
	if len(first.CodeActions) != 1 {
		t.Fatalf("expected one code action preview, got %#v", first.CodeActions)
	}
	var got []string
	for _, actions := range first.CodeActions {
		got = actions
	}
	if len(got) != 1 || got[0] != "apply fix" {
		t.Fatalf("expected code action preview, got %#v", first.CodeActions)
	}

	second, err := service.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch(second): %v", err)
	}
	if !second.Found {
		t.Fatal("expected second fetch to still return an entry")
	}
	if len(second.Entry.Diagnostics) != 0 {
		t.Fatalf("expected surfaced dedup to suppress repeat diagnostics, got %#v", second.Entry.Diagnostics)
	}
	if len(second.CodeActions) != 0 {
		t.Fatalf("expected no code actions after dedup, got %#v", second.CodeActions)
	}

	raw, err := service.Fetch(context.Background(), policy.DiagnosticsRequest{
		Path:         path,
		SessionID:    req.SessionID,
		Freshness:    policy.DiagnosticsFreshnessPeek,
		Presentation: policy.DiagnosticsPresentationRaw,
	})
	if err != nil {
		t.Fatalf("Fetch(raw): %v", err)
	}
	if len(raw.Entry.Diagnostics) != 1 || raw.Entry.Diagnostics[0].Message != "surface me" {
		t.Fatalf("expected raw presentation to keep the underlying diagnostic, got %#v", raw.Entry.Diagnostics)
	}
}

func TestDiagnosticsServiceSurfacedPresentationDedupIsSessionScoped(t *testing.T) {
	t.Parallel()

	diagnosticStore := store.New()
	engine := policy.New(config.Default().Policy, func(context.Context, string, protocol.Diagnostic) ([]string, error) {
		return []string{"apply fix"}, nil
	})
	service := policy.NewDiagnosticsService(nil, diagnosticStore, engine)

	path := t.TempDir() + "/session-scoped.go"
	uri := store.URIFromPath(path)
	diagnosticStore.Publish(uri, 1, []protocol.Diagnostic{{
		Message:  "session scoped",
		Severity: protocol.DiagnosticSeverityError,
	}}, "go")

	first, err := service.Fetch(context.Background(), policy.DiagnosticsRequest{
		Path:         path,
		SessionID:    "session-a",
		Freshness:    policy.DiagnosticsFreshnessPeek,
		Presentation: policy.DiagnosticsPresentationSurfaced,
	})
	if err != nil {
		t.Fatalf("Fetch(session-a): %v", err)
	}
	if !first.Found || len(first.Entry.Diagnostics) != 1 {
		t.Fatalf("expected initial surfaced diagnostics for session-a, got %#v", first.Entry.Diagnostics)
	}

	repeat, err := service.Fetch(context.Background(), policy.DiagnosticsRequest{
		Path:         path,
		SessionID:    "session-a",
		Freshness:    policy.DiagnosticsFreshnessPeek,
		Presentation: policy.DiagnosticsPresentationSurfaced,
	})
	if err != nil {
		t.Fatalf("Fetch(session-a repeat): %v", err)
	}
	if len(repeat.Entry.Diagnostics) != 0 {
		t.Fatalf("expected repeat session-a fetch to be deduped, got %#v", repeat.Entry.Diagnostics)
	}

	secondSession, err := service.Fetch(context.Background(), policy.DiagnosticsRequest{
		Path:         path,
		SessionID:    "session-b",
		Freshness:    policy.DiagnosticsFreshnessPeek,
		Presentation: policy.DiagnosticsPresentationSurfaced,
	})
	if err != nil {
		t.Fatalf("Fetch(session-b): %v", err)
	}
	if !secondSession.Found || len(secondSession.Entry.Diagnostics) != 1 {
		t.Fatalf("expected surfaced diagnostics to reappear for a new session, got %#v", secondSession.Entry.Diagnostics)
	}
	if len(secondSession.CodeActions) != 1 {
		t.Fatalf("expected code actions to remain available for a new session, got %#v", secondSession.CodeActions)
	}
}
