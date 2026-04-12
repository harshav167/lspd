package mocklsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.lsp.dev/protocol"
)

type ServeOptions struct {
	RecordFile         string                `json:"record_file,omitempty"`
	CrashMarkerFile    string                `json:"crash_marker_file,omitempty"`
	CrashAfterOpenOnce bool                  `json:"crash_after_open_once,omitempty"`
	PublishOnOpen      []protocol.Diagnostic `json:"publish_on_open,omitempty"`
	PublishOnChange    []protocol.Diagnostic `json:"publish_on_change,omitempty"`
}

type requestEnvelope struct {
	ID     any             `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type didOpenParams struct {
	TextDocument protocol.TextDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument protocol.VersionedTextDocumentIdentifier  `json:"textDocument"`
	Content      []protocol.TextDocumentContentChangeEvent `json:"contentChanges"`
}

// Serve responds to a small LSP subset used by resilience tests.
func Serve(ctx context.Context, reader io.Reader, writer io.Writer, options ServeOptions) error {
	buffered := bufio.NewReader(reader)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := buffered.ReadString('\n')
		if err != nil {
			return err
		}
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "content-length:") {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(lower), ":", 2)
		if len(parts) != 2 {
			continue
		}
		length, parseErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if parseErr != nil || length <= 0 {
			continue
		}
		if _, err := buffered.ReadString('\n'); err != nil {
			return fmt.Errorf("read header terminator: %w", err)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(buffered, body); err != nil {
			return err
		}
		var request requestEnvelope
		if err := json.Unmarshal(body, &request); err != nil {
			return err
		}
		if err := recordMethod(options.RecordFile, request.Method); err != nil {
			return err
		}
		switch request.Method {
		case protocol.MethodInitialize:
			response := map[string]any{
				"capabilities": map[string]any{
					"documentSymbolProvider":  true,
					"workspaceSymbolProvider": true,
					"definitionProvider":      true,
				},
			}
			if err := writeResult(writer, request.ID, response); err != nil {
				return err
			}
		case protocol.MethodInitialized:
		case protocol.MethodWorkspaceSymbol:
			if err := writeResult(writer, request.ID, []any{}); err != nil {
				return err
			}
		case protocol.MethodShutdown:
			if err := writeResult(writer, request.ID, nil); err != nil {
				return err
			}
		case protocol.MethodExit:
			return nil
		case protocol.MethodTextDocumentDidOpen:
			var params didOpenParams
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return err
			}
			if err := writeDiagnostics(writer, params.TextDocument.URI, params.TextDocument.Version, options.PublishOnOpen); err != nil {
				return err
			}
			if options.CrashAfterOpenOnce && shouldCrash(options.CrashMarkerFile) {
				return errors.New("mocklsp forced crash")
			}
		case protocol.MethodTextDocumentDidChange:
			var params didChangeParams
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return err
			}
			if err := writeDiagnostics(writer, params.TextDocument.URI, params.TextDocument.Version, diagnosticsForChange(options)); err != nil {
				return err
			}
		default:
			if request.ID == nil {
				continue
			}
			if err := writeResult(writer, request.ID, nil); err != nil {
				return err
			}
		}
	}
}

func diagnosticsForChange(options ServeOptions) []protocol.Diagnostic {
	if len(options.PublishOnChange) > 0 {
		return options.PublishOnChange
	}
	return options.PublishOnOpen
}

func writeResult(writer io.Writer, id any, result any) error {
	response := map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

func writeDiagnostics(writer io.Writer, uri protocol.DocumentURI, version int32, diagnostics []protocol.Diagnostic) error {
	if diagnostics == nil {
		diagnostics = []protocol.Diagnostic{}
	}
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  protocol.MethodTextDocumentPublishDiagnostics,
		"params": map[string]any{
			"uri":         uri,
			"version":     version,
			"diagnostics": diagnostics,
		},
	}
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

func recordMethod(path, method string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, method)
	return err
}

func shouldCrash(marker string) bool {
	if marker == "" {
		return true
	}
	if _, err := os.Stat(marker); err == nil {
		return false
	}
	_ = os.MkdirAll(filepath.Dir(marker), 0o755)
	return os.WriteFile(marker, []byte("crashed"), 0o600) == nil
}
