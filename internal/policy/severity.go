package policy

import (
	"sort"

	"github.com/harsha/lspd/internal/config"
	"go.lsp.dev/protocol"
)

func filterDiagnostics(cfg config.PolicyConfig, diagnostics []protocol.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if !allowSeverity(cfg.MinimumSeverity, diagnostic) || !allowSource(cfg, diagnostic.Source) {
			continue
		}
		out = append(out, diagnostic)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func allowSeverity(minimum int, diagnostic protocol.Diagnostic) bool {
	if diagnostic.Severity == 0 {
		return true
	}
	return int(diagnostic.Severity) <= minimum+1
}

func allowSource(cfg config.PolicyConfig, source string) bool {
	if len(cfg.AllowedSources) > 0 {
		allowed := false
		for _, item := range cfg.AllowedSources {
			if item == source {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	for _, item := range cfg.DeniedSources {
		if item == source {
			return false
		}
	}
	return true
}
