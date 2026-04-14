package socket

import (
	"time"

	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/policy"
)

// Request is the unix socket RPC request shape.
type Request struct {
	Op           string                         `json:"op"`
	Path         string                         `json:"path,omitempty"`
	URI          string                         `json:"uri,omitempty"`
	SessionID    string                         `json:"session_id,omitempty"`
	Kind         string                         `json:"kind,omitempty"`
	TimeoutMs    int                            `json:"timeout_ms,omitempty"`
	MinSeverity  string                         `json:"min_severity,omitempty"`
	Freshness    policy.DiagnosticsFreshness    `json:"freshness,omitempty"`
	Presentation policy.DiagnosticsPresentation `json:"presentation,omitempty"`
}

// Response is the unix socket RPC response shape.
type Response struct {
	OK          bool                `json:"ok"`
	Message     string              `json:"message,omitempty"`
	Entry       *store.Entry        `json:"entry,omitempty"`
	CodeActions map[string][]string `json:"code_actions,omitempty"`
	Entries     []store.Entry       `json:"entries,omitempty"`
	Status      map[string]any      `json:"status,omitempty"`
	Time        time.Time           `json:"time,omitempty"`
}

// DiagnosticsFreshness resolves the request freshness semantics with transport-compatible defaults.
func (r Request) DiagnosticsFreshness() policy.DiagnosticsFreshness {
	if r.Freshness != "" {
		return r.Freshness
	}
	switch r.Op {
	case "peek":
		return policy.DiagnosticsFreshnessPeek
	case "fetch":
		return policy.DiagnosticsFreshnessBestEffortNow
	case "drain":
		if r.Kind == "read" {
			return policy.DiagnosticsFreshnessBestEffortNow
		}
		return policy.DiagnosticsFreshnessDrain
	default:
		return policy.DiagnosticsFreshnessPeek
	}
}

// DiagnosticsPresentation resolves the request presentation semantics with surfaced defaults.
func (r Request) DiagnosticsPresentation() policy.DiagnosticsPresentation {
	if r.Presentation != "" {
		return r.Presentation
	}
	return policy.DiagnosticsPresentationSurfaced
}
