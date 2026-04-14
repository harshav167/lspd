package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/format"
	"github.com/harshav167/lspd/internal/policy"
	"github.com/harshav167/lspd/internal/socket"
)

type hookInput struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Filepath string `json:"filepath"`
		Path     string `json:"path"`
	} `json:"tool_input"`
}

const (
	connectTimeout = 500 * time.Millisecond
	requestTimeout = 2 * time.Second
)

func main() {
	log.SetOutput(io.Discard)
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		writeHookOutput("")
		return
	}
	if input.ToolName != "" && input.ToolName != "Read" {
		writeHookOutput("")
		return
	}
	path := hookPath(input)
	if path == "" {
		writeHookOutput("")
		return
	}
	// This binary is wired only into PostToolUse(Read). Writes come from Droid's
	// native IDE auto-connect diagnostics path; this hook only surfaces read-time
	// reminders from lspd's diagnostic store.
	response, err := request(socket.Request{
		Op:           "fetch",
		Path:         path,
		SessionID:    input.SessionID,
		Kind:         "read",
		TimeoutMs:    int(requestTimeout / time.Millisecond),
		Freshness:    policy.DiagnosticsFreshnessBestEffortNow,
		Presentation: policy.DiagnosticsPresentationSurfaced,
	})
	if err != nil || response.Entry == nil {
		writeHookOutput("")
		return
	}
	reminder := format.SystemReminder(path, response.Entry.Diagnostics, response.CodeActions)
	writeHookOutput(reminder)
}

func request(req socket.Request) (socket.Response, error) {
	conn, err := net.DialTimeout("unix", socketPath(), connectTimeout)
	if err != nil {
		return socket.Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return socket.Response{}, err
	}
	var response socket.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return socket.Response{}, err
	}
	return response, nil
}

func socketPath() string {
	if value := os.Getenv("LSPD_SOCKET_PATH"); value != "" {
		return value
	}
	return config.Default().Socket.Path
}

func hookPath(input hookInput) string {
	for _, candidate := range []string{input.ToolInput.FilePath, input.ToolInput.Filepath, input.ToolInput.Path} {
		if candidate != "" {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func writeHookOutput(additionalContext string) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"suppressOutput": true,
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "PostToolUse",
			"additionalContext": additionalContext,
		},
	})
}
