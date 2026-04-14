package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/socket"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lspd <start|stop|status|reload|logs|diag|fix|forget|ping|version>")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "start":
		err = runStart(os.Args[2:])
	case "stop":
		err = runStop(os.Args[2:])
	case "status":
		err = runStatus(os.Args[2:])
	case "reload":
		err = runReload(os.Args[2:])
	case "logs":
		err = runLogs(os.Args[2:])
	case "diag":
		err = runDiag(os.Args[2:])
	case "fix":
		err = runFix(os.Args[2:])
	case "forget":
		err = runForget(os.Args[2:])
	case "ping":
		err = runPing(os.Args[2:])
	case "version":
		fmt.Println("lspd " + version)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func requestSocket(path string, request socket.Request) (socket.Response, error) {
	conn, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return socket.Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return socket.Response{}, err
	}
	var response socket.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return socket.Response{}, err
	}
	if !response.OK && response.Message != "" {
		return response, fmt.Errorf("%s", response.Message)
	}
	return response, nil
}

func addConfigFlag(flags *flag.FlagSet) *string {
	return flags.String("config", os.Getenv("LSPD_CONFIG"), "path to lspd config")
}

func loadCLIConfig(explicitPath string) (config.Config, error) {
	cfg, _, err := config.Load(explicitPath, mustGetwd())
	if err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func requestConfiguredSocket(explicitPath string, request socket.Request) (socket.Response, error) {
	cfg, err := loadCLIConfig(explicitPath)
	if err != nil {
		return socket.Response{}, err
	}
	return requestSocket(cfg.Socket.Path, request)
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func sliceValue(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

func stringSliceValue(value any) []string {
	items := sliceValue(value)
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := stringValue(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func parseRFC3339Time(value any) (time.Time, bool) {
	text := stringValue(value)
	if text == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func relativeTime(value any) string {
	parsed, ok := parseRFC3339Time(value)
	if !ok || parsed.IsZero() {
		return ""
	}
	return time.Since(parsed).Round(time.Second).String() + " ago"
}
