package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	internalmcp "github.com/harsha/lspd/internal/mcp"
	"github.com/harsha/lspd/internal/policy"
	"go.lsp.dev/protocol"
)

type diagnosticsPayload struct {
	Diagnostics []struct {
		Severity int    `json:"severity"`
		Message  string `json:"message"`
	} `json:"diagnostics"`
}

func integrationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
}

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func TestMCPContractBoots(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Normalize()
	diagnosticStore := store.New()
	server := internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: nil,
		Router: router.New(cfg, diagnosticStore, nil, nil),
		Store:  diagnosticStore,
		Policy: policy.New(cfg.Policy, nil),
		Logger: nil,
	})
	port, err := server.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if port == 0 {
		t.Fatal("expected non-zero port")
	}

	client, err := mcpclient.NewStreamableHttpClient("http://127.0.0.1:" + strconv.Itoa(port) + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHttpClient: %v", err)
	}
	defer client.Close()
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start client: %v", err)
	}
	if _, err := client.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "e2e", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("Initialize client: %v", err)
	}

	toolList, err := client.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	expectedTools := map[string]struct{}{
		"getIdeDiagnostics": {}, "openDiff": {}, "closeDiff": {}, "openFile": {},
		"lspDefinition": {}, "lspReferences": {}, "lspHover": {}, "lspWorkspaceSymbol": {},
		"lspDocumentSymbol": {}, "lspCodeActions": {}, "lspRename": {}, "lspFormat": {},
		"lspCallHierarchy": {}, "lspTypeHierarchy": {},
	}
	for _, tool := range toolList.Tools {
		delete(expectedTools, tool.Name)
	}
	if len(expectedTools) != 0 {
		t.Fatalf("missing tools: %+v", expectedTools)
	}

	tmp := filepath.Join(t.TempDir(), "example.ts")
	if err := os.WriteFile(tmp, []byte("const x = 1\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	diagResult, err := client.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "getIdeDiagnostics",
			Arguments: map[string]any{"uri": "file://" + filepath.ToSlash(tmp)},
		},
	})
	if err != nil {
		t.Fatalf("CallTool getIdeDiagnostics: %v", err)
	}
	if diagResult.IsError {
		t.Fatal("expected getIdeDiagnostics success")
	}
	for _, name := range []string{"openDiff", "closeDiff", "openFile"} {
		result, callErr := client.CallTool(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name}})
		if callErr != nil {
			t.Fatalf("CallTool %s: %v", name, callErr)
		}
		if result.IsError {
			t.Fatalf("expected %s stub success", name)
		}
	}
	toolArgs := []mcp.CallToolRequest{
		{Params: mcp.CallToolParams{Name: "lspDefinition", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1}}},
		{Params: mcp.CallToolParams{Name: "lspReferences", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1, "include_declaration": true}}},
		{Params: mcp.CallToolParams{Name: "lspHover", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1}}},
		{Params: mcp.CallToolParams{Name: "lspWorkspaceSymbol", Arguments: map[string]any{"query": "Add", "path": filepath.Join(t.TempDir(), "missing.go")}}},
		{Params: mcp.CallToolParams{Name: "lspDocumentSymbol", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go")}}},
		{Params: mcp.CallToolParams{Name: "lspCodeActions", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "start_line": 1, "start_character": 1, "end_line": 1, "end_character": 2}}},
		{Params: mcp.CallToolParams{Name: "lspRename", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1, "new_name": "Renamed", "dry_run": true}}},
		{Params: mcp.CallToolParams{Name: "lspFormat", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go")}}},
		{Params: mcp.CallToolParams{Name: "lspCallHierarchy", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1, "direction": "incoming"}}},
		{Params: mcp.CallToolParams{Name: "lspTypeHierarchy", Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "missing.go"), "line": 1, "character": 1, "direction": "super"}}},
	}
	for _, request := range toolArgs {
		result, callErr := client.CallTool(context.Background(), request)
		if callErr != nil {
			t.Fatalf("CallTool %s: %v", request.Params.Name, callErr)
		}
		if request.Params.Name == "lspWorkspaceSymbol" {
			if result.IsError {
				t.Fatalf("expected %s to gracefully degrade for invalid input, got %#v", request.Params.Name, result.Content)
			}
			continue
		}
		if !result.IsError {
			t.Fatalf("expected %s to return a tool-visible error for invalid input", request.Params.Name)
		}
	}

	if err := server.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestGetIdeDiagnosticsUsesStickyMCPSessionAndCachedFallback(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Normalize()
	diagnosticStore := store.New()
	server := internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: nil,
		Router: router.New(cfg, diagnosticStore, integrationLogger(), nil),
		Store:  diagnosticStore,
		Policy: policy.New(cfg.Policy, nil),
		Logger: integrationLogger(),
	})
	port, err := server.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if closeErr := server.Close(context.Background()); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	}()

	tempPath := filepath.Join(t.TempDir(), "cached.txt")
	if err := os.WriteFile(tempPath, []byte("cached\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	uri := "file://" + filepath.ToSlash(tempPath)
	diagnosticStore.Publish(protocol.DocumentURI(uri), 1, []protocol.Diagnostic{{
		Message:  "cached failure",
		Severity: protocol.DiagnosticSeverityError,
		Source:   "unit",
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 4},
			End:   protocol.Position{Line: 2, Character: 9},
		},
	}}, "unit")

	newClient := func(t *testing.T) *mcpclient.Client {
		t.Helper()
		client, err := mcpclient.NewStreamableHttpClient("http://127.0.0.1:" + strconv.Itoa(port) + "/mcp")
		if err != nil {
			t.Fatalf("NewStreamableHttpClient: %v", err)
		}
		if err := client.Start(context.Background()); err != nil {
			t.Fatalf("Start client: %v", err)
		}
		if _, err := client.Initialize(context.Background(), mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "e2e", Version: "1.0.0"},
			},
		}); err != nil {
			t.Fatalf("Initialize client: %v", err)
		}
		return client
	}

	callDiagnostics := func(t *testing.T, client *mcpclient.Client, uri string) diagnosticsPayload {
		t.Helper()
		result, err := client.CallTool(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      "getIdeDiagnostics",
				Arguments: map[string]any{"uri": uri},
			},
		})
		if err != nil {
			t.Fatalf("CallTool getIdeDiagnostics: %v", err)
		}
		if result.IsError {
			t.Fatal("expected getIdeDiagnostics success")
		}
		if len(result.Content) != 1 {
			t.Fatalf("expected a single content item, got %d", len(result.Content))
		}
		textContent, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("expected text content, got %#v", result.Content[0])
		}
		var response diagnosticsPayload
		if err := json.Unmarshal([]byte(textContent.Text), &response); err != nil {
			t.Fatalf("Unmarshal diagnostics response: %v", err)
		}
		return response
	}

	client1 := newClient(t)
	defer client1.Close()

	first := callDiagnostics(t, client1, uri)
	if len(first.Diagnostics) != 1 {
		t.Fatalf("expected first call to surface cached diagnostic, got %+v", first.Diagnostics)
	}
	if first.Diagnostics[0].Severity != 0 || first.Diagnostics[0].Message != "cached failure" {
		t.Fatalf("unexpected diagnostic payload: %+v", first.Diagnostics[0])
	}

	second := callDiagnostics(t, client1, uri)
	if len(second.Diagnostics) != 0 {
		t.Fatalf("expected second call in same MCP session to dedupe diagnostics, got %+v", second.Diagnostics)
	}

	client2 := newClient(t)
	defer client2.Close()

	third := callDiagnostics(t, client2, uri)
	if len(third.Diagnostics) != 1 {
		t.Fatalf("expected a new MCP session to surface diagnostics again, got %+v", third.Diagnostics)
	}

	invalid := callDiagnostics(t, client2, "bad://uri")
	if len(invalid.Diagnostics) != 0 {
		t.Fatalf("expected invalid URI to degrade to empty diagnostics, got %+v", invalid.Diagnostics)
	}
}

func TestNavigationWireContractsExerciseAllTier2Tools(t *testing.T) {
	t.Parallel()
	requireBinary(t, "gopls")

	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/navwire\n\ngo 1.22\n")
	libContent := "package lib\n\n// Add sums two integers.\nfunc Add(a int, b int) int { return a + b }\n"
	mainContent := "package main\n\nimport \"example.com/navwire/lib\"\n\ntype Adder interface {\n\tAdd(a int, b int) int\n}\n\ntype myAdder struct{}\n\nfunc (myAdder) Add(a int, b int) int {\n\treturn lib.Add(a, b)\n}\n\nfunc wrapper() int {\n\treturn lib.Add( 1, 2 )\n}\n\nfunc main() {\n\t_ = wrapper()\n}\n"
	brokenContent := "package main\n\nfunc broken() {\n\t_ = fmt.Println(\"hi\")\n}\n"
	libPath := writeFile(t, root, "lib/lib.go", libContent)
	mainPath := writeFile(t, root, "main.go", mainContent)
	brokenPath := writeFile(t, root, "broken.go", brokenContent)

	cfg := config.Default()
	cfg.Normalize()
	diagnosticStore := store.New()
	server := internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: nil,
		Router: router.New(cfg, diagnosticStore, integrationLogger(), nil),
		Store:  diagnosticStore,
		Policy: policy.New(cfg.Policy, nil),
		Logger: integrationLogger(),
	})
	port, err := server.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if closeErr := server.Close(context.Background()); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	}()

	client, err := mcpclient.NewStreamableHttpClient("http://127.0.0.1:" + strconv.Itoa(port) + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHttpClient: %v", err)
	}
	defer client.Close()
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start client: %v", err)
	}
	if _, err := client.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "nav-wire", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("Initialize client: %v", err)
	}

	callStructured := func(name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := client.CallTool(context.Background(), mcp.CallToolRequest{
			Params: mcp.CallToolParams{Name: name, Arguments: args},
		})
		if err != nil {
			t.Fatalf("CallTool %s: %v", name, err)
		}
		if result.IsError {
			t.Fatalf("CallTool %s returned error: %#v", name, result.Content)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		return payload
	}

	lineNumberForToken := func(content, token string) int {
		lines := strings.Split(content, "\n")
		for idx, line := range lines {
			if strings.Contains(line, token) {
				return idx + 1
			}
		}
		return 0
	}
	columnForToken := func(content string, line int, token string) int {
		lines := strings.Split(content, "\n")
		if line <= 0 || line > len(lines) {
			return 0
		}
		return strings.Index(lines[line-1], token) + 1
	}

	addDocLine := lineNumberForToken(libContent, "Add sums two integers.")
	addDefLine := addDocLine + 1
	addCol := columnForToken(libContent, addDefLine, "Add")

	definition := callStructured("lspDefinition", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "lib.Add( 1, 2 )"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "lib.Add( 1, 2 )"), "Add"),
	})
	if defs, ok := definition["definitions"].([]any); !ok || len(defs) == 0 {
		t.Fatalf("expected definitions, got %#v", definition)
	}

	references := callStructured("lspReferences", map[string]any{
		"path":                libPath,
		"line":                addDefLine,
		"character":           addCol,
		"include_declaration": true,
	})
	if total, ok := references["total"].(float64); !ok || total < 3 {
		t.Fatalf("expected references total >= 3, got %#v", references["total"])
	}

	hover := callStructured("lspHover", map[string]any{
		"path":      libPath,
		"line":      addDefLine,
		"character": addCol,
	})
	if signature, _ := hover["type_signature"].(string); !strings.Contains(signature, "func Add") {
		t.Fatalf("expected hover type signature, got %#v", hover["type_signature"])
	}

	workspace := callStructured("lspWorkspaceSymbol", map[string]any{
		"query": "wrapper",
		"path":  mainPath,
	})
	if symbols, ok := workspace["symbols"].([]any); !ok || len(symbols) == 0 {
		t.Fatalf("expected workspace symbols, got %#v", workspace)
	}

	document := callStructured("lspDocumentSymbol", map[string]any{"path": mainPath})
	if symbols, ok := document["symbols"].([]any); !ok || len(symbols) == 0 {
		t.Fatalf("expected document symbols, got %#v", document)
	}

	codeActions := callStructured("lspCodeActions", map[string]any{
		"path":            brokenPath,
		"start_line":      lineNumberForToken(brokenContent, "fmt.Println"),
		"start_character": columnForToken(brokenContent, lineNumberForToken(brokenContent, "fmt.Println"), "fmt"),
		"end_line":        lineNumberForToken(brokenContent, "fmt.Println"),
		"end_character":   columnForToken(brokenContent, lineNumberForToken(brokenContent, "fmt.Println"), "fmt") + len("fmt"),
	})
	if actions, ok := codeActions["actions"].([]any); !ok || len(actions) == 0 {
		t.Fatalf("expected code actions, got %#v", codeActions)
	}

	rename := callStructured("lspRename", map[string]any{
		"path":      libPath,
		"line":      addDefLine,
		"character": addCol,
		"new_name":  "Sum",
		"dry_run":   true,
	})
	if filesTouched, _ := rename["files_touched"].(float64); filesTouched < 2 {
		t.Fatalf("expected rename to touch multiple files, got %#v", rename["files_touched"])
	}

	formatResult := callStructured("lspFormat", map[string]any{"path": mainPath})
	if changed, _ := formatResult["changed"].(bool); !changed {
		t.Fatalf("expected formatting to change file, got %#v", formatResult)
	}

	callHierarchy := callStructured("lspCallHierarchy", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "func wrapper() int"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "func wrapper() int"), "wrapper"),
		"direction": "outgoing",
	})
	if calls, ok := callHierarchy["calls"].([]any); !ok || len(calls) == 0 {
		t.Fatalf("expected call hierarchy edges, got %#v", callHierarchy)
	}

	typeHierarchy := callStructured("lspTypeHierarchy", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "type Adder interface"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "type Adder interface"), "Adder"),
		"direction": "sub",
	})
	if _, ok := typeHierarchy["types"].([]any); !ok {
		t.Fatalf("expected type hierarchy result, got %#v", typeHierarchy)
	}
}
