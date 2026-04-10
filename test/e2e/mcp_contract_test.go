package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	internalmcp "github.com/harsha/lspd/internal/mcp"
	"github.com/harsha/lspd/internal/policy"
)

func TestMCPContractBoots(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	diagnosticStore := store.New()
	server := internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: nil,
		Router: router.New(cfg, diagnosticStore, nil),
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
		if !result.IsError {
			t.Fatalf("expected %s to return a tool-visible error for invalid input", request.Params.Name)
		}
	}

	if err := server.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
