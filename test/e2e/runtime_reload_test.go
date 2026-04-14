package e2e

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/harshav167/lspd/internal/config"
)

func TestConfigReloadHonestyClassifiesAppliedAndDeferredChanges(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "lspd.yaml")
	writeConfig := func(contents string) {
		t.Helper()
		if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	writeConfig("policy:\n  minimum_severity: 1\nrun_dir: ./run-a\n")
	manager, err := config.NewManager(configPath, root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	initial := manager.Metadata()

	writeConfig("policy:\n  minimum_severity: 2\nrun_dir: ./run-b\n")
	_, report, err := manager.Reload(context.Background())
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if !report.Changed {
		t.Fatal("expected reload to report a config change")
	}
	if !slices.Contains(report.AppliedNow, "diagnostic policy") {
		t.Fatalf("expected policy change to apply now, got %#v", report.AppliedNow)
	}
	if !slices.Contains(report.Deferred, "runtime directories and idle policy") {
		t.Fatalf("expected runtime directory change to be deferred, got %#v", report.Deferred)
	}
	message := report.Message()
	if !strings.Contains(message, "reload applied now: diagnostic policy") {
		t.Fatalf("expected applied-now message, got %q", message)
	}
	if !strings.Contains(message, "deferred until restart: runtime directories and idle policy") {
		t.Fatalf("expected deferred message, got %q", message)
	}

	reloaded := manager.Metadata()
	if reloaded.Generation != initial.Generation+1 {
		t.Fatalf("expected config generation to advance, got %d then %d", initial.Generation, reloaded.Generation)
	}
	if !reloaded.ReloadedAt.After(initial.ReloadedAt) {
		t.Fatalf("expected reload timestamp to advance, got %v then %v", initial.ReloadedAt, reloaded.ReloadedAt)
	}
}
