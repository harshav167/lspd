package socket

import (
	"time"

	"github.com/harsha/lspd/internal/lsp/store"
)

// Request is the unix socket RPC request shape.
type Request struct {
	Op          string `json:"op"`
	Path        string `json:"path,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Kind        string `json:"kind,omitempty"`
	TimeoutMs   int    `json:"timeout_ms,omitempty"`
	MinSeverity string `json:"min_severity,omitempty"`
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
