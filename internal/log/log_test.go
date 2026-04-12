package log

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/harsha/lspd/internal/config"
)

func TestLevelForRespectsConfig(t *testing.T) {
	t.Parallel()

	if got := levelFor(config.Config{LogLevel: "debug"}); got != slog.LevelDebug {
		t.Fatalf("expected debug level, got %v", got)
	}
	if got := levelFor(config.Config{LogLevel: "warn"}); got != slog.LevelWarn {
		t.Fatalf("expected warn level, got %v", got)
	}
	if got := levelFor(config.Config{Debug: true}); got != slog.LevelDebug {
		t.Fatalf("expected debug level from debug flag, got %v", got)
	}
}

func TestWithReturnsUsableLoggerWhenNil(t *testing.T) {
	t.Parallel()

	logger := With(nil, "component", "test")
	if logger == nil {
		t.Fatal("expected logger")
	}
}

func TestNewReturnsLogger(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.LogFile = filepath.Join(t.TempDir(), "lspd.log")
	logger := New(cfg)
	if logger == nil {
		t.Fatal("expected logger")
	}
}
