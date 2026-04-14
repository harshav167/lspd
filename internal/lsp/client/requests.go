package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.lsp.dev/protocol"
)

// TypeHierarchyItem is a lightweight type hierarchy payload for raw LSP methods.
type TypeHierarchyItem struct {
	Name           string         `json:"name"`
	Kind           float64        `json:"kind"`
	Detail         string         `json:"detail,omitempty"`
	URI            string         `json:"uri"`
	Range          protocol.Range `json:"range"`
	SelectionRange protocol.Range `json:"selectionRange"`
	Data           any            `json:"data,omitempty"`
}

func (m *Manager) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, m.requestTimeout)
}

func (m *Manager) didOpen(ctx context.Context, doc Document) error {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        doc.URI,
			LanguageID: doc.LanguageID,
			Version:    doc.Version,
			Text:       doc.Content,
		},
	})
}

func (m *Manager) didChange(ctx context.Context, doc Document) error {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			Version:                doc.Version,
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: doc.URI},
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: doc.Content}},
	})
}

// EnsureOpen opens or updates a document in the language server.
func (m *Manager) EnsureOpen(ctx context.Context, path string) (Document, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	content := string(contentBytes)
	uri := pathToURI(path)
	doc, ok := m.tracker.get(uri)
	if !ok {
		doc = Document{
			Path:         path,
			URI:          uri,
			LanguageID:   m.cfg.LanguageID,
			Version:      1,
			Content:      content,
			LastAccessed: time.Now(),
		}
		if err := m.didOpen(ctx, doc); err != nil {
			return Document{}, err
		}
	} else if doc.Content != content {
		doc.Version++
		doc.Content = content
		doc.LastAccessed = time.Now()
		if err := m.didChange(ctx, doc); err != nil {
			return Document{}, err
		}
	} else {
		doc.LastAccessed = time.Now()
	}
	m.tracker.put(doc)
	return doc, nil
}

// RestoreDocument re-registers a tracked document on a fresh language server.
func (m *Manager) RestoreDocument(ctx context.Context, doc Document) error {
	_, err := m.EnsureOpen(ctx, doc.Path)
	return err
}

// Close closes a tracked document.
func (m *Manager) Close(ctx context.Context, uri protocol.DocumentURI) error {
	if _, ok := m.tracker.get(uri); !ok {
		return nil
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	if err := m.server.DidClose(ctx, &protocol.DidCloseTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}}); err != nil {
		return err
	}
	m.tracker.delete(uri)
	return nil
}

// Definition returns definition locations for a symbol.
func (m *Manager) Definition(ctx context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.Definition(ctx, params)
}

// References returns references for a symbol.
func (m *Manager) References(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.References(ctx, params)
}

// Hover returns hover information.
func (m *Manager) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.Hover(ctx, params)
}

// DocumentSymbol returns document symbols.
func (m *Manager) DocumentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) ([]interface{}, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.DocumentSymbol(ctx, params)
}

// WorkspaceSymbol returns workspace symbols.
func (m *Manager) WorkspaceSymbol(ctx context.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.Symbols(ctx, params)
}

// CodeAction returns code actions for the provided range and diagnostics.
func (m *Manager) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.CodeAction(ctx, params)
}

// Rename prepares a workspace edit for a rename.
func (m *Manager) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.Rename(ctx, params)
}

// Formatting formats a document.
func (m *Manager) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.Formatting(ctx, params)
}

// PrepareCallHierarchy prepares the call hierarchy at a position.
func (m *Manager) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.PrepareCallHierarchy(ctx, params)
}

// IncomingCalls returns incoming call hierarchy edges.
func (m *Manager) IncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.IncomingCalls(ctx, params)
}

// OutgoingCalls returns outgoing call hierarchy edges.
func (m *Manager) OutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.OutgoingCalls(ctx, params)
}

// PrepareTypeHierarchy prepares type hierarchy items.
func (m *Manager) PrepareTypeHierarchy(ctx context.Context, params map[string]any) ([]TypeHierarchyItem, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	var result []TypeHierarchyItem
	if _, err := m.conn.Call(ctx, "textDocument/prepareTypeHierarchy", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Supertypes returns type supertypes.
func (m *Manager) Supertypes(ctx context.Context, item TypeHierarchyItem) ([]TypeHierarchyItem, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	var result []TypeHierarchyItem
	if _, err := m.conn.Call(ctx, "typeHierarchy/supertypes", map[string]any{"item": item}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Subtypes returns type subtypes.
func (m *Manager) Subtypes(ctx context.Context, item TypeHierarchyItem) ([]TypeHierarchyItem, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	var result []TypeHierarchyItem
	if _, err := m.conn.Call(ctx, "typeHierarchy/subtypes", map[string]any{"item": item}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Save notifies the language server that a document was saved.
func (m *Manager) Save(ctx context.Context, uri protocol.DocumentURI, text string) error {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()
	return m.server.DidSave(ctx, &protocol.DidSaveTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}, Text: text})
}

func encodeRange(rangeValue protocol.Range) string {
	data, _ := json.Marshal(rangeValue)
	return string(data)
}

func (m *Manager) String() string {
	return fmt.Sprintf("manager{%s:%s}", m.cfg.Name, m.root)
}
