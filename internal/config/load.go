package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from the default search path or an explicit path.
func Load(explicitPath string, cwd string) (Config, string, error) {
	cfg := Default()
	paths := candidatePaths(explicitPath, cwd)
	loadedFrom := ""
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Config{}, "", fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, "", fmt.Errorf("decode config %s: %w", path, err)
		}
		loadedFrom = path
		break
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, loadedFrom, err
	}
	return cfg, loadedFrom, nil
}

func candidatePaths(explicitPath string, cwd string) []string {
	if explicitPath != "" {
		return []string{explicitPath}
	}
	home, _ := os.UserHomeDir()
	paths := []string{filepath.Join(home, ".config", "lspd", "config.yaml")}
	dir := cwd
	for dir != "" && dir != "/" {
		paths = append(paths, filepath.Join(dir, "lspd.yaml"))
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return paths
}
