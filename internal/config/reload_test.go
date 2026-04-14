package config

import "testing"

func cloneLanguages(in map[string]LanguageConfig) map[string]LanguageConfig {
	out := make(map[string]LanguageConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func TestClassifyReloadMarksPolicyAsAppliedNow(t *testing.T) {
	t.Parallel()

	current := Default()
	next := current
	next.Policy.MaxPerTurn = current.Policy.MaxPerTurn + 5

	report := classifyReload(current, next, "/tmp/a.yaml", "/tmp/a.yaml")
	if !report.Changed {
		t.Fatal("expected reload report to mark a config change")
	}
	if len(report.AppliedNow) != 1 || report.AppliedNow[0] != "diagnostic policy" {
		t.Fatalf("unexpected applied-now entries: %#v", report.AppliedNow)
	}
	if len(report.Deferred) != 0 {
		t.Fatalf("expected no deferred entries, got %#v", report.Deferred)
	}
}

func TestClassifyReloadMarksLanguageChangesAsMixedRuntimeAndDeferred(t *testing.T) {
	t.Parallel()

	current := Default()
	next := current
	next.Languages = cloneLanguages(current.Languages)
	lang := next.Languages["go"]
	lang.Command = "custom-gopls"
	next.Languages["go"] = lang

	report := classifyReload(current, next, "/tmp/a.yaml", "/tmp/a.yaml")
	if !report.Changed {
		t.Fatal("expected reload report to mark a config change")
	}
	if len(report.AppliedNow) != 1 || report.AppliedNow[0] != "language routing for newly resolved files" {
		t.Fatalf("unexpected applied-now entries: %#v", report.AppliedNow)
	}
	if len(report.Deferred) != 1 || report.Deferred[0] != "already-running language server sessions keep prior command/settings until restart" {
		t.Fatalf("unexpected deferred entries: %#v", report.Deferred)
	}
}
