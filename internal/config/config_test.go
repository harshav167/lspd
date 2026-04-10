package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrefersExplicitPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("mcp:\n  endpoint: /mcp\nlanguages:\n  go:\n    command: gopls\n    extensions: ['.go']\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, loadedFrom, err := Load(configPath, root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loadedFrom != configPath {
		t.Fatalf("expected loaded path %s, got %s", configPath, loadedFrom)
	}
	if cfg.MCP.Endpoint != "/mcp" {
		t.Fatalf("unexpected endpoint %q", cfg.MCP.Endpoint)
	}
}

func TestValidateRejectsMissingCommand(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.Languages["bad"] = LanguageConfig{Extensions: []string{".bad"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
