package client

import (
	"context"
	"path/filepath"
	"time"

	"go.lsp.dev/protocol"
)

func (m *Manager) initialize(ctx context.Context) error {
	params := &protocol.InitializeParams{
		ProcessID: int32(0),
		RootURI:   pathToURI(m.root),
		RootPath:  m.root,
		Capabilities: protocol.ClientCapabilities{
			Workspace: &protocol.WorkspaceClientCapabilities{
				WorkspaceFolders:       m.cfg.WorkspaceFolders,
				Configuration:          true,
				DidChangeConfiguration: &protocol.DidChangeConfigurationWorkspaceClientCapabilities{DynamicRegistration: true},
			},
			TextDocument: &protocol.TextDocumentClientCapabilities{
				Synchronization:    &protocol.TextDocumentSyncClientCapabilities{DidSave: true, WillSave: true},
				PublishDiagnostics: &protocol.PublishDiagnosticsClientCapabilities{RelatedInformation: true},
				Definition:         &protocol.DefinitionTextDocumentClientCapabilities{LinkSupport: true},
				TypeDefinition:     &protocol.TypeDefinitionTextDocumentClientCapabilities{LinkSupport: true},
				References:         &protocol.ReferencesTextDocumentClientCapabilities{DynamicRegistration: true},
				Hover:              &protocol.HoverTextDocumentClientCapabilities{ContentFormat: []protocol.MarkupKind{protocol.Markdown, protocol.PlainText}},
				DocumentSymbol:     &protocol.DocumentSymbolClientCapabilities{HierarchicalDocumentSymbolSupport: true},
				CodeAction:         &protocol.CodeActionClientCapabilities{IsPreferredSupport: true},
				Rename:             &protocol.RenameClientCapabilities{PrepareSupport: true},
				CallHierarchy:      &protocol.CallHierarchyClientCapabilities{DynamicRegistration: true},
			},
		},
		InitializationOptions: m.cfg.InitializationOptions,
		WorkspaceFolders: []protocol.WorkspaceFolder{{
			URI:  string(pathToURI(m.root)),
			Name: filepath.Base(m.root),
		}},
	}
	if _, err := m.server.Initialize(ctx, params); err != nil {
		return err
	}
	if err := m.server.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		return err
	}
	if m.cfg.Settings != nil {
		if err := m.server.DidChangeConfiguration(ctx, &protocol.DidChangeConfigurationParams{Settings: m.cfg.Settings}); err != nil {
			return err
		}
	}
	if m.cfg.Warmup {
		go m.warmup()
	}
	return nil
}

func (m *Manager) warmup() {
	timeout := m.requestTimeout
	if timeout < 10*time.Second {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(m.runCtx, timeout)
	defer cancel()
	if _, err := m.server.Symbols(ctx, &protocol.WorkspaceSymbolParams{Query: ""}); err != nil && ctx.Err() == nil {
		if m.logger != nil {
			m.logger.Debug("lsp warmup failed", "language", m.cfg.Name, "root", m.root, "error", err)
		}
	}
}

func pathToURI(path string) protocol.DocumentURI {
	return protocol.DocumentURI("file://" + filepath.ToSlash(path))
}
