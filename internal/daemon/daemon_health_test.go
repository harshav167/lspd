package daemon

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/metrics"
)

func TestHealthChecksDetectMissingSocketAndLock(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.RunDir = dir
	cfg.Socket.Path = filepath.Join(dir, "lspd.sock")
	cfg.MCP.Host = "127.0.0.1"

	app := &App{
		Config:      cfg,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:     metrics.New(),
		port:        1,
		ideLockPath: filepath.Join(dir, "missing.lock"),
		healthFails: map[string]int{},
	}

	if app.socketHealthy() {
		t.Fatal("expected missing socket to be unhealthy")
	}
	if app.ideLockHealthy() {
		t.Fatal("expected missing lock file to be unhealthy")
	}
}

func TestHealthChecksDetectPresentSocketAndLock(t *testing.T) {
	dir := filepath.Join("/tmp", "lspd-health-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}
	cfg := config.Default()
	cfg.RunDir = dir
	cfg.Socket.Path = filepath.Join(dir, "lspd.sock")

	lockPath := filepath.Join(dir, "123.lock")
	if err := os.WriteFile(lockPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	listener, err := netListenUnix(cfg.Socket.Path)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer listener.Close()

	app := &App{
		Config:      cfg,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:     metrics.New(),
		ideLockPath: lockPath,
		healthFails: map[string]int{},
	}

	if !app.socketHealthy() {
		t.Fatal("expected socket to be healthy")
	}
	if !app.ideLockHealthy() {
		t.Fatal("expected lock file to be healthy")
	}
}

func netListenUnix(path string) (io.Closer, error) {
	return net.Listen("unix", path)
}

func TestCheckHealthCancelsAfterThreeFailures(t *testing.T) {
	cfg := config.Default()
	cfg.Socket.Path = filepath.Join(t.TempDir(), "missing.sock")
	app := &App{
		Config:      cfg,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:     metrics.New(),
		healthFails: map[string]int{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.cancel = cancel

	for i := 0; i < 3; i++ {
		app.checkHealth(ctx)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected cancel after repeated health failures")
	}
}

func TestTouchIdleUpdatesTimer(t *testing.T) {
	app := &App{idleTimer: newIdleTimer(time.Second)}
	before := app.idleTimer.last
	time.Sleep(10 * time.Millisecond)
	app.touchIdle()
	if !app.idleTimer.last.After(before) {
		t.Fatal("expected idle timer to be touched")
	}
}
