package client

import (
	"context"
	"path/filepath"

	"go.lsp.dev/protocol"
)

func (m *Manager) initialize(ctx context.Context) error {
	params := &protocol.InitializeParams{
		ProcessID:             int32(0),
		RootURI:               pathToURI(m.root),
		RootPath:              m.root,
		Capabilities:          protocol.ClientCapabilities{},
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
	return nil
}

func pathToURI(path string) protocol.DocumentURI {
	return protocol.DocumentURI("file://" + filepath.ToSlash(path))
}
