package nav

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/harsha/lspd/internal/format"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	"github.com/harsha/lspd/internal/metrics"
	"github.com/harsha/lspd/internal/policy"
	sdkmcp "github.com/mark3labs/mcp-go/mcp"
	"go.lsp.dev/protocol"
)

// Dependencies are shared across navigation handlers.
type Dependencies struct {
	Router  *router.Router
	Store   *store.Store
	Policy  *policy.Engine
	Metrics *metrics.Registry
}

type positionArgs struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

type rangeArgs struct {
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	StartCharacter int    `json:"start_character"`
	EndLine        int    `json:"end_line"`
	EndCharacter   int    `json:"end_character"`
}

type typeHierarchyItem = client.TypeHierarchyItem

type editPlan struct {
	Changes []editChange `json:"changes"`
}

type editChange struct {
	Path    string       `json:"path"`
	Range   format.Range `json:"range"`
	NewText string       `json:"new_text"`
}

type commandSummary struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

func resolvePosition(ctx context.Context, deps Dependencies, args positionArgs) (Dependencies, protocol.TextDocumentPositionParams, error) {
	manager, _, err := deps.Router.Resolve(ctx, args.Path)
	if err != nil {
		return deps, protocol.TextDocumentPositionParams{}, err
	}
	doc, err := manager.EnsureOpen(ctx, args.Path)
	if err != nil {
		return deps, protocol.TextDocumentPositionParams{}, err
	}
	_ = doc
	return deps, protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentURI("file://" + filepath.ToSlash(args.Path))},
		Position:     protocol.Position{Line: uint32(max(args.Line-1, 0)), Character: uint32(max(args.Character-1, 0))},
	}, nil
}

func responseJSON[T any](payload T) (*sdkmcp.CallToolResult, error) {
	return sdkmcp.NewToolResultJSON(payload)
}

func recordToolRequest(deps Dependencies, name string) {
	if deps.Metrics != nil {
		deps.Metrics.RecordRequest("mcp", name)
	}
}

func pathFromURI(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return filepath.Clean(parsed.Path)
}

func documentURI(path string) protocol.DocumentURI {
	return protocol.DocumentURI("file://" + filepath.ToSlash(path))
}

func locationFromProtocol(location protocol.Location) format.Location {
	path := pathFromURI(string(location.URI))
	return format.Location{
		Path:      path,
		Line:      int(location.Range.Start.Line) + 1,
		Column:    int(location.Range.Start.Character) + 1,
		EndLine:   int(location.Range.End.Line) + 1,
		EndColumn: int(location.Range.End.Character) + 1,
		Preview:   format.PreviewLine(path, int(location.Range.Start.Line)),
	}
}

func markdownToText(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case protocol.MarkupContent:
		return typed.Value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func symbolKindName(kind protocol.SymbolKind) string {
	switch kind {
	case protocol.SymbolKindFile:
		return "file"
	case protocol.SymbolKindModule:
		return "module"
	case protocol.SymbolKindNamespace:
		return "namespace"
	case protocol.SymbolKindPackage:
		return "package"
	case protocol.SymbolKindClass:
		return "class"
	case protocol.SymbolKindMethod:
		return "method"
	case protocol.SymbolKindProperty:
		return "property"
	case protocol.SymbolKindField:
		return "field"
	case protocol.SymbolKindConstructor:
		return "constructor"
	case protocol.SymbolKindEnum:
		return "enum"
	case protocol.SymbolKindInterface:
		return "interface"
	case protocol.SymbolKindFunction:
		return "function"
	case protocol.SymbolKindVariable:
		return "variable"
	case protocol.SymbolKindConstant:
		return "constant"
	case protocol.SymbolKindString:
		return "string"
	case protocol.SymbolKindNumber:
		return "number"
	case protocol.SymbolKindBoolean:
		return "boolean"
	case protocol.SymbolKindArray:
		return "array"
	case protocol.SymbolKindObject:
		return "object"
	case protocol.SymbolKindKey:
		return "key"
	case protocol.SymbolKindNull:
		return "null"
	case protocol.SymbolKindEnumMember:
		return "enum_member"
	case protocol.SymbolKindStruct:
		return "struct"
	case protocol.SymbolKindEvent:
		return "event"
	case protocol.SymbolKindOperator:
		return "operator"
	case protocol.SymbolKindTypeParameter:
		return "type_parameter"
	default:
		return fmt.Sprintf("kind_%d", int(kind))
	}
}

func toFormatRange(rangeValue protocol.Range) format.Range {
	return format.Range{
		Start: format.Position{Line: int(rangeValue.Start.Line) + 1, Column: int(rangeValue.Start.Character) + 1},
		End:   format.Position{Line: int(rangeValue.End.Line) + 1, Column: int(rangeValue.End.Character) + 1},
	}
}

func decodeInto[T any](value interface{}, dest *T) bool {
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

func workspaceEditChanges(edit *protocol.WorkspaceEdit) []editChange {
	if edit == nil {
		return nil
	}
	changes := make([]editChange, 0)
	appendTextEdits := func(path string, edits []protocol.TextEdit) {
		for _, edit := range edits {
			changes = append(changes, editChange{
				Path:    path,
				Range:   toFormatRange(edit.Range),
				NewText: edit.NewText,
			})
		}
	}
	for rawURI, edits := range edit.Changes {
		appendTextEdits(pathFromURI(string(rawURI)), edits)
	}
	for _, documentChange := range edit.DocumentChanges {
		appendTextEdits(pathFromURI(string(documentChange.TextDocument.URI)), documentChange.Edits)
	}
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		if changes[i].Range.Start.Line != changes[j].Range.Start.Line {
			return changes[i].Range.Start.Line < changes[j].Range.Start.Line
		}
		return changes[i].Range.Start.Column < changes[j].Range.Start.Column
	})
	return changes
}

func workspaceEditSummary(edit *protocol.WorkspaceEdit) (*editPlan, int, int) {
	changes := workspaceEditChanges(edit)
	if len(changes) == 0 {
		return nil, 0, 0
	}
	files := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		files[change.Path] = struct{}{}
	}
	return &editPlan{Changes: changes}, len(files), len(changes)
}

func sortLocations(locations []format.Location) {
	sort.SliceStable(locations, func(i, j int) bool {
		if locations[i].Path != locations[j].Path {
			return locations[i].Path < locations[j].Path
		}
		if locations[i].Line != locations[j].Line {
			return locations[i].Line < locations[j].Line
		}
		if locations[i].Column != locations[j].Column {
			return locations[i].Column < locations[j].Column
		}
		if locations[i].EndLine != locations[j].EndLine {
			return locations[i].EndLine < locations[j].EndLine
		}
		return locations[i].EndColumn < locations[j].EndColumn
	})
}

func applyTextEdits(content string, edits []protocol.TextEdit) string {
	if len(edits) == 0 {
		return content
	}
	lines := splitKeepTrailing(content)
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Range.Start.Line != edits[j].Range.Start.Line {
			return edits[i].Range.Start.Line > edits[j].Range.Start.Line
		}
		return edits[i].Range.Start.Character > edits[j].Range.Start.Character
	})
	for _, edit := range edits {
		startLine := int(edit.Range.Start.Line)
		endLine := int(edit.Range.End.Line)
		if startLine < 0 || endLine >= len(lines) || startLine > endLine {
			continue
		}
		startChar := int(edit.Range.Start.Character)
		endChar := int(edit.Range.End.Character)
		prefix := safeSliceUTF16(lines[startLine], 0, startChar)
		suffix := safeSliceUTF16(lines[endLine], endChar, utf16Len(lines[endLine]))
		replacement := strings.Split(edit.NewText, "\n")
		if len(replacement) == 1 {
			lines[startLine] = prefix + replacement[0] + suffix
			lines = append(lines[:startLine+1], lines[endLine+1:]...)
			continue
		}
		replacement[0] = prefix + replacement[0]
		replacement[len(replacement)-1] = replacement[len(replacement)-1] + suffix
		lines = append(lines[:startLine], append(replacement, lines[endLine+1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func splitKeepTrailing(content string) []string {
	if content == "" {
		return []string{""}
	}
	return strings.Split(content, "\n")
}

func safeSliceUTF16(value string, start, end int) string {
	runes := []rune(value)
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	startIdx := utf16OffsetToRuneIndex(runes, start)
	endIdx := utf16OffsetToRuneIndex(runes, end)
	if endIdx < startIdx {
		endIdx = startIdx
	}
	return string(runes[startIdx:endIdx])
}

func utf16Len(value string) int {
	length := 0
	for _, r := range value {
		if r > 0xFFFF {
			length += 2
			continue
		}
		length++
	}
	return length
}

func utf16OffsetToRuneIndex(runes []rune, offset int) int {
	if offset <= 0 {
		return 0
	}
	units := 0
	for idx, r := range runes {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if units >= offset {
			return idx
		}
		if units+width > offset {
			return idx + 1
		}
		units += width
	}
	return len(runes)
}

func identifierAtPosition(path string, line, character int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	runes := []rune(lines[line-1])
	index := character - 1
	if index < 0 {
		index = 0
	}
	if index > len(runes) {
		index = len(runes)
	}
	start := index
	if start == len(runes) && start > 0 {
		start--
	}
	for start > 0 && isIdentifierRune(runes[start-1]) {
		start--
	}
	end := index
	if end < len(runes) && !isIdentifierRune(runes[end]) && end > start {
		end--
	}
	for end < len(runes) && isIdentifierRune(runes[end]) {
		end++
	}
	if start >= end || start < 0 || end > len(runes) {
		return ""
	}
	return string(runes[start:end])
}

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
