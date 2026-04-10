//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/store"
)

func integrationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
}

func startManager(t *testing.T, lang config.LanguageConfig, root string) (*client.Manager, *store.Store, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	diagnosticStore := store.New()
	manager, err := client.NewManager(ctx, lang, root, diagnosticStore, integrationLogger())
	if err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager, diagnosticStore, ctx
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
