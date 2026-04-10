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
	if c.Socket.Path == "" {
		return errors.New("socket.path is required")
	}
	if c.MCP.Endpoint == "" || c.MCP.Endpoint[0] != '/' {
		return errors.New("mcp.endpoint must start with /")
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
