package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookPathPrefersFirstAvailablePath(t *testing.T) {
	t.Parallel()

	input := hookInput{}
	input.ToolInput.Filepath = "./second"
	input.ToolInput.Path = "./third"
	input.ToolInput.FilePath = "./first"

	got := hookPath(input)
	if got != filepath.Clean("./first") {
		t.Fatalf("expected first path candidate, got %q", got)
	}
}

func TestSocketPathUsesEnvOverride(t *testing.T) {
	t.Parallel()

	const want = "/tmp/custom-lspd.sock"
	if err := os.Setenv("LSPD_SOCKET_PATH", want); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("LSPD_SOCKET_PATH") })

	if got := socketPath(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestWriteHookOutputEncodesPostToolUsePayload(t *testing.T) {
	t.Parallel()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = original
		_ = reader.Close()
	})

	writeHookOutput("diagnostics here")
	_ = writer.Close()

	var payload map[string]any
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if suppress, _ := payload["suppressOutput"].(bool); !suppress {
		t.Fatalf("expected suppressOutput=true, got %#v", payload["suppressOutput"])
	}
	hookSpecific, ok := payload["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("expected hookSpecificOutput object, got %#v", payload["hookSpecificOutput"])
	}
	if event, _ := hookSpecific["hookEventName"].(string); event != "PostToolUse" {
		t.Fatalf("expected PostToolUse event, got %#v", hookSpecific["hookEventName"])
	}
	if context, _ := hookSpecific["additionalContext"].(string); !strings.Contains(context, "diagnostics here") {
		t.Fatalf("expected diagnostics text in payload, got %#v", hookSpecific["additionalContext"])
	}
}
