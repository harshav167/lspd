//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	internalmcp "github.com/harshav167/lspd/internal/mcp"
	"github.com/harshav167/lspd/internal/policy"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

func TestNavigationContracts(t *testing.T) {
	requireBinary(t, "gopls")
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/nav\n\ngo 1.22\n")
	libContent := "package lib\n\n// Add sums two integers.\nfunc Add(a int, b int) int { return a + b }\n"
	mainContent := "package main\n\nimport \"example.com/nav/lib\"\n\ntype Adder interface {\n\tAdd(a int, b int) int\n}\n\ntype myAdder struct{}\n\nfunc (myAdder) Add(a int, b int) int {\n\treturn lib.Add(a, b)\n}\n\nfunc wrapper() int {\n\treturn lib.Add( 1, 2 )\n}\n\nfunc main() {\n\t_ = wrapper()\n}\n"
	brokenContent := "package main\n\nfunc broken() {\n\t_ = fmt.Println(\"hi\")\n}\n"
	libPath := writeFile(t, root, "lib/lib.go", libContent)
	mainPath := writeFile(t, root, "main.go", mainContent)
	brokenPath := writeFile(t, root, "broken.go", brokenContent)

	cfg := config.Default().Languages["go"]
	manager, _, ctx := startManager(t, cfg, root)
	if _, err := manager.EnsureOpen(ctx, mainPath); err != nil {
		t.Fatalf("EnsureOpen: %v", err)
	}
	addLine := lineNumberForToken(libContent, "Add sums two integers.")
	addDefLine := addLine + 1
	addCol := columnForToken(libContent, addDefLine, "Add")
	defs, err := manager.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + mainPath)},
			Position: protocol.Position{
				Line:      uint32(lineNumberForToken(mainContent, "lib.Add( 1, 2 )") - 1),
				Character: uint32(columnForToken(mainContent, lineNumberForToken(mainContent, "lib.Add( 1, 2 )"), "Add") - 1),
			},
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected definition result")
	}
	refs, err := manager.References(ctx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: defs[0].URI},
			Position:     defs[0].Range.Start,
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("expected references result")
	}

	fullCfg := config.Default()
	fullCfg.Normalize()
	diagnosticStore := store.New()
	server := internalmcp.NewServer(fullCfg, internalmcp.Dependencies{
		Config: nil,
		Router: router.New(fullCfg, diagnosticStore, integrationLogger(), nil),
		Store:  diagnosticStore,
		Policy: policy.New(fullCfg.Policy, nil),
		Logger: integrationLogger(),
	})
	port, err := server.Start()
	if err != nil {
		t.Fatalf("Start MCP server: %v", err)
	}
	t.Cleanup(func() {
		_ = server.Close(context.Background())
	})

	client, err := mcpclient.NewStreamableHttpClient("http://127.0.0.1:" + strconv.Itoa(port) + "/mcp")
	if err != nil {
		t.Fatalf("NewStreamableHttpClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start client: %v", err)
	}
	if _, err := client.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "integration", Version: "1.0.0"},
		},
	}); err != nil {
		t.Fatalf("Initialize client: %v", err)
	}

	definitionResult := callStructuredTool(t, client, "lspDefinition", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "lib.Add( 1, 2 )"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "lib.Add( 1, 2 )"), "Add"),
	})
	definitions := sliceAt(t, definitionResult, "definitions")
	firstDefinition := mapAt(t, definitions[0])
	assertColumnOnly(t, firstDefinition)
	if firstDefinition["path"] != filepath.Clean(libPath) {
		t.Fatalf("expected definition path %s, got %#v", filepath.Clean(libPath), firstDefinition["path"])
	}
	if preview, _ := firstDefinition["preview"].(string); !strings.Contains(preview, "func Add") {
		t.Fatalf("expected definition preview, got %#v", firstDefinition["preview"])
	}

	referencesResult := callStructuredTool(t, client, "lspReferences", map[string]any{
		"path":                libPath,
		"line":                addDefLine,
		"character":           addCol,
		"include_declaration": true,
	})
	if value, ok := referencesResult["total"].(float64); !ok || value < 3 {
		t.Fatalf("expected at least 3 references, got %#v", referencesResult["total"])
	}
	byFile := sliceAt(t, referencesResult, "by_file")
	if len(byFile) < 2 {
		t.Fatalf("expected grouped reference summary, got %#v", referencesResult["by_file"])
	}
	firstReference := mapAt(t, sliceAt(t, referencesResult, "references")[0])
	assertColumnOnly(t, firstReference)
	if preview, _ := firstReference["preview"].(string); preview == "" {
		t.Fatalf("expected reference preview, got %#v", firstReference)
	}

	hoverResult := callStructuredTool(t, client, "lspHover", map[string]any{
		"path":      libPath,
		"line":      addDefLine,
		"character": addCol,
	})
	if signature, _ := hoverResult["type_signature"].(string); !strings.Contains(signature, "func Add") {
		t.Fatalf("expected hover type signature, got %#v", hoverResult["type_signature"])
	}
	if documentation, _ := hoverResult["documentation"].(string); !strings.Contains(documentation, "sums two integers") {
		t.Fatalf("expected hover documentation, got %#v", hoverResult["documentation"])
	}

	workspaceResult := callStructuredTool(t, client, "lspWorkspaceSymbol", map[string]any{
		"query": "wrapper",
		"path":  mainPath,
	})
	firstSymbol := mapAt(t, sliceAt(t, workspaceResult, "symbols")[0])
	assertColumnOnly(t, firstSymbol)

	documentResult := callStructuredTool(t, client, "lspDocumentSymbol", map[string]any{
		"path": mainPath,
	})
	firstDocSymbol := mapAt(t, sliceAt(t, documentResult, "symbols")[0])
	assertColumnOnly(t, firstDocSymbol)

	codeActionsResult := callStructuredTool(t, client, "lspCodeActions", map[string]any{
		"path":            brokenPath,
		"start_line":      lineNumberForToken(brokenContent, "fmt.Println"),
		"start_character": columnForToken(brokenContent, lineNumberForToken(brokenContent, "fmt.Println"), "fmt"),
		"end_line":        lineNumberForToken(brokenContent, "fmt.Println"),
		"end_character":   columnForToken(brokenContent, lineNumberForToken(brokenContent, "fmt.Println"), "fmt") + len("fmt"),
	})
	actions := sliceAt(t, codeActionsResult, "actions")
	if len(actions) == 0 {
		t.Fatal("expected code actions result")
	}
	firstAction := mapAt(t, actions[0])
	if edit, ok := firstAction["edit"].(map[string]any); ok {
		changes := sliceAt(t, edit, "changes")
		if len(changes) == 0 {
			t.Fatalf("expected nested code action edit changes, got %#v", edit)
		}
	} else if _, ok := firstAction["command"].(map[string]any); !ok {
		t.Fatalf("expected code action to expose edit or command, got %#v", firstAction)
	}

	renameResult := callStructuredTool(t, client, "lspRename", map[string]any{
		"path":      libPath,
		"line":      addDefLine,
		"character": addCol,
		"new_name":  "Sum",
		"dry_run":   true,
	})
	if filesTouched, _ := renameResult["files_touched"].(float64); filesTouched < 2 {
		t.Fatalf("expected cross-file rename plan, got %#v", renameResult["files_touched"])
	}
	renameEdit := mapAt(t, renameResult["edit"])
	renameChanges := sliceAt(t, renameEdit, "changes")
	if len(renameChanges) < 2 {
		t.Fatalf("expected nested rename changes, got %#v", renameEdit)
	}
	if _, found := renameEdit["documentChanges"]; found {
		t.Fatalf("unexpected raw workspace edit payload: %#v", renameEdit)
	}

	formatResult := callStructuredTool(t, client, "lspFormat", map[string]any{"path": mainPath})
	if changed, _ := formatResult["changed"].(bool); !changed {
		t.Fatalf("expected formatter to change file, got %#v", formatResult)
	}
	if _, exists := formatResult["range"]; !exists {
		t.Fatalf("expected explicit null range for document formatting, got %#v", formatResult)
	}
	if formatted, _ := formatResult["new_text"].(string); !strings.Contains(formatted, "lib.Add(1, 2)") {
		t.Fatalf("expected formatted content, got %#v", formatResult["new_text"])
	}

	callHierarchyResult := callStructuredTool(t, client, "lspCallHierarchy", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "func wrapper() int"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "func wrapper() int"), "wrapper"),
		"direction": "outgoing",
	})
	if direction, _ := callHierarchyResult["direction"].(string); direction != "outgoing" {
		t.Fatalf("expected outgoing direction, got %#v", callHierarchyResult["direction"])
	}
	callItem := mapAt(t, callHierarchyResult["item"])
	assertColumnOnly(t, callItem)
	firstCall := mapAt(t, sliceAt(t, callHierarchyResult, "calls")[0])
	if _, ok := firstCall["to"].(map[string]any); !ok {
		t.Fatalf("expected outgoing hierarchy edge to expose callee, got %#v", firstCall)
	}
	callee := mapAt(t, firstCall["to"])
	if callee["path"] != filepath.Clean(libPath) {
		t.Fatalf("expected outgoing callee path %s, got %#v", filepath.Clean(libPath), callee["path"])
	}
	assertColumnOnly(t, callee)
	callSite := mapAt(t, sliceAt(t, firstCall, "call_sites")[0])
	if callSite["path"] != filepath.Clean(mainPath) {
		t.Fatalf("expected outgoing call site path to remain on caller file, got %#v", callSite["path"])
	}
	assertColumnOnly(t, callSite)

	typeHierarchyResult := callStructuredTool(t, client, "lspTypeHierarchy", map[string]any{
		"path":      mainPath,
		"line":      lineNumberForToken(mainContent, "type Adder interface"),
		"character": columnForToken(mainContent, lineNumberForToken(mainContent, "type Adder interface"), "Adder"),
		"direction": "sub",
	})
	if direction, _ := typeHierarchyResult["direction"].(string); direction != "sub" {
		t.Fatalf("expected subtype direction, got %#v", typeHierarchyResult["direction"])
	}
	typeItem := mapAt(t, typeHierarchyResult["item"])
	assertColumnOnly(t, typeItem)
	if _, ok := typeHierarchyResult["types"].([]any); !ok {
		t.Fatalf("expected type hierarchy types array, got %#v", typeHierarchyResult["types"])
	}
	typeItems := sliceAt(t, typeHierarchyResult, "types")
	if len(typeItems) == 0 {
		t.Fatalf("expected subtype results, got %#v", typeHierarchyResult["types"])
	}
	firstType := mapAt(t, typeItems[0])
	if firstType["path"] != filepath.Clean(mainPath) {
		t.Fatalf("expected subtype path %s, got %#v", filepath.Clean(mainPath), firstType["path"])
	}
	assertColumnOnly(t, firstType)
}

func callStructuredTool(t *testing.T, client *mcpclient.Client, name string, args map[string]any) map[string]any {
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
	var payload map[string]any
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal %s structured content: %v", name, err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal %s structured content: %v", name, err)
	}
	return payload
}

func lineNumberForToken(content, token string) int {
	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		if strings.Contains(line, token) {
			return idx + 1
		}
	}
	return 0
}

func columnForToken(content string, line int, token string) int {
	lines := strings.Split(content, "\n")
	if line <= 0 || line > len(lines) {
		return 0
	}
	return strings.Index(lines[line-1], token) + 1
}

func sliceAt(t *testing.T, value map[string]any, key string) []any {
	t.Helper()
	items, ok := value[key].([]any)
	if !ok {
		t.Fatalf("expected %s to be an array, got %#v", key, value[key])
	}
	return items
}

func mapAt(t *testing.T, value any) map[string]any {
	t.Helper()
	item, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %#v", value)
	}
	return item
}

func assertColumnOnly(t *testing.T, value map[string]any) {
	t.Helper()
	if _, ok := value["column"]; !ok {
		t.Fatalf("expected column field, got %#v", value)
	}
	if _, found := value["character"]; found {
		t.Fatalf("unexpected character field in %#v", value)
	}
}
