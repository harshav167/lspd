package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestSessionStartScriptEmitsContext(t *testing.T) {
	home := testHome(t)
	lspd := fakeLSPD(t, home)

	// Create the config directory and a minimal config file
	configDir := filepath.Join(home, ".factory", "hooks", "lsp")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configFile := filepath.Join(configDir, "lspd.yaml")
	if err := os.WriteFile(configFile, []byte("run_dir: ~/.factory/run/lspd\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "session-start.sh"))
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+filepath.Dir(lspd)+":"+os.Getenv("PATH"),
		"LSPD_CONFIG="+configFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("session-start failed: %v\n%s", err, output)
	}
	defer stopLSPD(home, lspd)

	if !strings.Contains(string(output), "\"hookEventName\":\"SessionStart\"") {
		t.Fatalf("expected SessionStart hook output, got %q", string(output))
	}
	if !strings.Contains(string(output), "LSP bridge active") {
		t.Fatalf("expected additionalContext with LSP bridge info, got %q", string(output))
	}
}

func TestDroidLauncherExportsPortToRealDroid(t *testing.T) {
	home := testHome(t)
	lspd := fakeLSPD(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".factory", "run", "droid-lsp"), 0o755); err != nil {
		t.Fatalf("mkdir launcher run dir: %v", err)
	}

	fakeDroid := filepath.Join(home, "fake-droid.sh")
	if err := os.WriteFile(fakeDroid, []byte("#!/usr/bin/env sh\nprintf 'PORT=%s\\nARGS=%s\\n' \"$FACTORY_VSCODE_MCP_PORT\" \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("write fake droid: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "droid-launcher.sh"), "--version")
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"LSPD_BIN="+lspd,
		"REAL_DROID="+fakeDroid,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher failed: %v\n%s", err, output)
	}
	defer stopLSPD(home, lspd)

	lines := string(output)
	if !strings.Contains(lines, "PORT=") {
		t.Fatalf("expected launcher to export FACTORY_VSCODE_MCP_PORT, got %q", lines)
	}
	expectedSettings := filepath.Join(home, ".local", "bin", "droid-lsp-settings.json")
	if !strings.Contains(lines, "ARGS=--settings "+expectedSettings+" --version") {
		t.Fatalf("expected launcher to pass through args, got %q", lines)
	}
}

func testHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0o755); err != nil {
		t.Fatalf("mkdir test home: %v", err)
	}
	return home
}

func fakeLSPD(t *testing.T, home string) string {
	t.Helper()
	binPath := filepath.Join(home, ".local", "bin", "lspd")
	// Fake lspd that skips --config flags to find the subcommand
	script := "#!/usr/bin/env sh\n" +
		"set -eu\n" +
		"STATE=\"$HOME/.factory/run/lspd.running\"\n" +
		"PORT_FILE=\"$HOME/.factory/run/lspd.port\"\n" +
		"mkdir -p \"$(dirname \"$STATE\")\"\n" +
		"# Skip --config <value> flags to find the subcommand\n" +
		"CMD=''\n" +
		"SKIP_NEXT=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ $SKIP_NEXT -eq 1 ]; then SKIP_NEXT=0; continue; fi\n" +
		"  case \"$arg\" in --config) SKIP_NEXT=1; continue;; --*) continue;; esac\n" +
		"  CMD=\"$arg\"; break\n" +
		"done\n" +
		"case \"${CMD:-}\" in\n" +
		"  ping)\n" +
		"    [ -f \"$STATE\" ] || exit 1\n" +
		"    printf 'pong\\n'\n" +
		"    ;;\n" +
		"  start)\n" +
		"    printf '48123' >\"$PORT_FILE\"\n" +
		"    : >\"$STATE\"\n" +
		"    printf '48123\\n'\n" +
		"    ;;\n" +
		"  stop)\n" +
		"    rm -f \"$STATE\" \"$PORT_FILE\"\n" +
		"    ;;\n" +
		"  *)\n" +
		"    exit 1\n" +
		"    ;;\n" +
		"esac\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake lspd: %v", err)
	}
	return binPath
}

func stopLSPD(home, lspd string) {
	cmd := exec.Command(lspd, "stop")
	cmd.Env = append(os.Environ(), "HOME="+home)
	_ = cmd.Run()
}
