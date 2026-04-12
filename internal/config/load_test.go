package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesUserAndProjectConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	home := filepath.Join(filepath.Clean(getenv(t, "HOME")), ".factory", "hooks", "lsp")
	project := t.TempDir()
	overrideDir := filepath.Join(project, ".factory", "lsp")

	mustWriteFile(t, filepath.Join(home, "lspd.yaml"), "debug: true\npolicy:\n  max_per_file: 11\n")
	mustWriteFile(t, filepath.Join(overrideDir, "lspd.yaml"), "policy:\n  max_per_file: 7\nwatcher:\n  enabled: false\n")

	cfg, loadedFrom, err := Load("", project)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Debug {
		t.Fatal("expected user config to set debug")
	}
	if cfg.Policy.MaxPerFile != 7 {
		t.Fatalf("expected project override to win, got %d", cfg.Policy.MaxPerFile)
	}
	if cfg.Watcher.Enabled {
		t.Fatal("expected project override to disable watcher")
	}
	if !strings.Contains(loadedFrom, filepath.Join(home, "lspd.yaml")) {
		t.Fatalf("expected loadedFrom to include user config, got %q", loadedFrom)
	}
	if !strings.Contains(loadedFrom, filepath.Join(overrideDir, "lspd.yaml")) {
		t.Fatalf("expected loadedFrom to include project override, got %q", loadedFrom)
	}
}

func TestLoadExpandsPathsAndDeepMergesLanguages(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()
	overrideDir := filepath.Join(project, ".factory", "lsp")
	mustWriteFile(t, filepath.Join(overrideDir, "lspd.yaml"), strings.Join([]string{
		"run_dir: ~/custom-run",
		"log_file: ~/custom-logs/lspd.log",
		"languages:",
		"  ts:",
		"    warmup: false",
		"    settings:",
		"      typescript:",
		"        tsserver:",
		"          logVerbosity: verbose",
	}, "\n")+"\n")

	cfg, _, err := Load("", project)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := filepath.Join(getenv(t, "HOME"), "custom-run"); cfg.RunDir != want {
		t.Fatalf("expected expanded run dir %q, got %q", want, cfg.RunDir)
	}
	if want := filepath.Join(getenv(t, "HOME"), "custom-logs", "lspd.log"); cfg.LogFile != want {
		t.Fatalf("expected expanded log file %q, got %q", want, cfg.LogFile)
	}
	ts := cfg.Languages["ts"]
	if ts.Command != "typescript-language-server" {
		t.Fatalf("expected ts command to be preserved, got %q", ts.Command)
	}
	if ts.Warmup {
		t.Fatal("expected project override to disable ts warmup")
	}
	typescriptSettings, ok := ts.Settings["typescript"].(map[string]any)
	if !ok {
		t.Fatalf("expected typescript settings map, got %#v", ts.Settings["typescript"])
	}
	tsserverSettings, ok := typescriptSettings["tsserver"].(map[string]any)
	if !ok || tsserverSettings["logVerbosity"] != "verbose" {
		t.Fatalf("expected nested tsserver settings to be merged, got %#v", typescriptSettings["tsserver"])
	}
}

func TestLoadPreservesExplicitZeroIdleTimeout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	project := t.TempDir()
	overrideDir := filepath.Join(project, ".factory", "lsp")
	mustWriteFile(t, filepath.Join(overrideDir, "lspd.yaml"), "idle_timeout: 0s\n")

	cfg, _, err := Load("", project)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IdleTimeout.Duration != 0 {
		t.Fatalf("expected explicit zero idle timeout to be preserved, got %s", cfg.IdleTimeout.Duration)
	}
}

func getenv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("expected %s to be set", key)
	}
	return value
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
