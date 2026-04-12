//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	"github.com/harsha/lspd/test/mocklsp"
	"go.lsp.dev/protocol"
)

const mockLSPOptionsEnv = "LSPD_MOCK_LSP_OPTIONS"

func TestMockLSPHelperProcess(t *testing.T) {
	if os.Getenv(mockLSPOptionsEnv) == "" {
		return
	}
	var options mocklsp.ServeOptions
	encoded := os.Getenv(mockLSPOptionsEnv)
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if err := json.Unmarshal(payload, &options); err != nil {
		t.Fatalf("unmarshal options: %v", err)
	}
	if err := mocklsp.Serve(context.Background(), os.Stdin, os.Stdout, options); err != nil && !errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
	os.Exit(0)
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

func startManager(t *testing.T, lang config.LanguageConfig, root string) (*client.Manager, *store.Store, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	diagnosticStore := store.New()
	manager, err := client.NewManager(ctx, lang, root, diagnosticStore, integrationLogger(), nil)
	if err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager, diagnosticStore, ctx
}

func startRouter(t *testing.T, cfg config.Config) (*router.Router, *store.Store, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	diagnosticStore := store.New()
	instance := router.New(cfg, diagnosticStore, integrationLogger(), nil)
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	return instance, diagnosticStore, ctx
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

func openDocumentAndWait(t *testing.T, manager *client.Manager, diagnosticStore *store.Store, ctx context.Context, path string, save bool) (clientDocVersion int32, entry store.Entry) {
	t.Helper()
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		t.Fatalf("EnsureOpen(%s): %v", path, err)
	}
	if save {
		if err := manager.Save(ctx, doc.URI, doc.Content); err != nil {
			t.Fatalf("Save(%s): %v", path, err)
		}
	}
	entry, ok, waitErr := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 10*time.Second)
	if waitErr != nil {
		t.Fatalf("Wait(%s): %v", path, waitErr)
	}
	if !ok {
		t.Fatalf("no diagnostics entry for %s", path)
	}
	if len(entry.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics for %s", path)
	}
	return doc.Version, entry
}

func updateDocumentAndWait(t *testing.T, manager *client.Manager, diagnosticStore *store.Store, ctx context.Context, path, content string, save bool) (clientDocVersion int32, entry store.Entry) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}
	return openDocumentAndWait(t, manager, diagnosticStore, ctx, path, save)
}

func mockLanguageConfig(t *testing.T, options mocklsp.ServeOptions, warmup bool) config.LanguageConfig {
	t.Helper()
	payload, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal mock options: %v", err)
	}
	return config.LanguageConfig{
		Name:             "mock",
		Command:          os.Args[0],
		Args:             []string{"-test.run=TestMockLSPHelperProcess"},
		Env:              map[string]string{mockLSPOptionsEnv: base64.StdEncoding.EncodeToString(payload)},
		Extensions:       []string{".mock"},
		RootMarkers:      []string{".git"},
		Settings:         map[string]any{},
		WorkspaceFolders: true,
		Warmup:           warmup,
		MaxRestarts:      2,
		RestartWindow:    config.Duration{Duration: 5 * time.Minute},
		DocumentTTL:      config.Duration{Duration: 5 * time.Minute},
		LanguageID:       protocol.LanguageIdentifier("mock"),
	}
}

func mockRouterConfig(t *testing.T, options mocklsp.ServeOptions, warmup bool) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Languages = map[string]config.LanguageConfig{
		"mock": mockLanguageConfig(t, options, warmup),
	}
	cfg.Normalize()
	return cfg
}

func requireEventually(t *testing.T, timeout, interval time.Duration, fn func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatal(message)
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), needle)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
