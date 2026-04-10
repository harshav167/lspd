package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStartScriptExportsPort(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	portFile := filepath.Join(tempDir, "lspd.port")
	envFile := filepath.Join(tempDir, "env.sh")
	lspd := filepath.Join(os.Getenv("HOME"), ".local", "bin", "lspd")

	cmd := exec.Command("sh", "/Users/harsha/.factory/droid-lsp/scripts/session-start.sh")
	cmd.Env = append(os.Environ(),
		"LSPD_BIN="+lspd,
		"LSPD_PORT_FILE="+portFile,
		"CLAUDE_ENV_FILE="+envFile,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("session-start failed: %v\n%s", err, output)
	}
	defer exec.Command(lspd, "stop").Run()

	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if !strings.Contains(string(data), "FACTORY_VSCODE_MCP_PORT=") {
		t.Fatalf("expected FACTORY_VSCODE_MCP_PORT export, got %q", string(data))
	}
}
