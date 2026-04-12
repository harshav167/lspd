package log

import (
	"io"
	stdlog "log"
	"log/slog"
	"os"

	"github.com/harsha/lspd/internal/config"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// New creates the daemon logger.
func New(cfg config.Config) *slog.Logger {
	level := levelFor(cfg)
	writer := io.Writer(&lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    cfg.LogMaxSizeMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAge:     cfg.LogMaxAgeDays,
	})
	var handler slog.Handler
	switch cfg.LogFormat {
	case "text":
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	stdlog.SetOutput(writer)
	return logger
}

// With returns a child logger.
func With(logger *slog.Logger, args ...any) *slog.Logger {
	if logger == nil {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)).With(args...)
	}
	return logger.With(args...)
}

func levelFor(cfg config.Config) slog.Level {
	switch cfg.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		if cfg.Debug {
			return slog.LevelDebug
		}
		return slog.LevelInfo
	}
}
