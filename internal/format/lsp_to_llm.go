package format

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
)

// Position is the 1-indexed tool surface position.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Range is the 1-indexed tool surface range.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// IdeDiagnostic mirrors Droid's expected diagnostic shape.
type IdeDiagnostic struct {
	Severity int             `json:"severity"`
	Message  string          `json:"message"`
	Source   string          `json:"source,omitempty"`
	Range    protocol.Range  `json:"range"`
	Code     json.RawMessage `json:"code,omitempty"`
}

// Location is an LLM-friendly source location.
type Location struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line,omitempty"`
	EndColumn int    `json:"end_column,omitempty"`
	Preview   string `json:"preview,omitempty"`
}

// ToIdeDiagnostics converts LSP diagnostics to Droid diagnostics.
func ToIdeDiagnostics(diagnostics []protocol.Diagnostic) []IdeDiagnostic {
	out := make([]IdeDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, IdeDiagnostic{Severity: toDroidSeverity(diagnostic.Severity), Message: diagnostic.Message, Source: diagnostic.Source, Range: diagnostic.Range, Code: marshalCode(diagnostic.Code)})
	}
	return out
}

func toDroidSeverity(severity protocol.DiagnosticSeverity) int {
	switch int(severity) {
	case 1:
		return 0
	case 2:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	default:
		return 0
	}
}

func marshalCode(code any) json.RawMessage {
	if code == nil {
		return nil
	}
	data, err := json.Marshal(code)
	if err != nil {
		return nil
	}
	return json.RawMessage(data)
}

// Fingerprint computes the stable policy fingerprint.
func Fingerprint(path string, diagnostic protocol.Diagnostic) string {
	if !strings.Contains(path, "://") {
		if abs, err := filepath.Abs(path); err == nil {
			path = (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
		}
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%v|%s|%s", path, diagnostic.Range.Start.Line, diagnostic.Range.Start.Character, diagnostic.Code, diagnostic.Source, diagnostic.Message)))
	return hex.EncodeToString(digest[:])
}

// PreviewLine returns the line at a location, if available.
func PreviewLine(path string, line int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line])
}
