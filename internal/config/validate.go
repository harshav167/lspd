package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Validate validates configuration values.
func (c Config) Validate() error {
	if c.RunDir == "" {
		return errors.New("run_dir is required")
	}
	if c.LogFile == "" {
		return errors.New("log_file is required")
	}
	if c.Socket.Path == "" {
		return errors.New("socket.path is required")
	}
	if c.MCP.Endpoint == "" || c.MCP.Endpoint[0] != '/' {
		return errors.New("mcp.endpoint must start with /")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be one of debug, info, warn, error")
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("log_format must be one of json, text")
	}
	if c.Metrics.Enabled && c.Metrics.Port <= 0 {
		return errors.New("metrics.port must be > 0 when metrics are enabled")
	}
	if len(c.Languages) == 0 {
		return errors.New("at least one language must be configured")
	}
	for name, lang := range c.Languages {
		if lang.Command == "" {
			return fmt.Errorf("language %s command is required", name)
		}
		if len(lang.Extensions) == 0 {
			return fmt.Errorf("language %s extensions are required", name)
		}
	}
	for ext, language := range c.LanguageByExt {
		if _, ok := c.Languages[language]; !ok {
			return fmt.Errorf("language_by_ext %s references unknown language %s", ext, language)
		}
	}
	for _, dir := range []string{c.RunDir, filepath.Dir(c.LogFile), filepath.Dir(c.Socket.Path)} {
		if dir == "" || dir == "." {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	return nil
}
