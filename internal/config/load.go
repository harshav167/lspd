package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load loads configuration from the default search path or an explicit path.
func Load(explicitPath string, cwd string) (Config, string, error) {
	base, err := configNode(Default())
	if err != nil {
		return Config{}, "", err
	}
	paths := candidatePaths(explicitPath, cwd)
	loadedFrom := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && explicitPath == "" {
				continue
			}
			return Config{}, "", fmt.Errorf("read config %s: %w", path, err)
		}
		overlay, err := decodeNode(data)
		if err != nil {
			return Config{}, "", fmt.Errorf("decode config %s: %w", path, err)
		}
		mergeNodes(base, overlay)
		loadedFrom = append(loadedFrom, path)
	}
	var cfg Config
	if err := base.Decode(&cfg); err != nil {
		return Config{}, strings.Join(loadedFrom, ","), fmt.Errorf("decode merged config: %w", err)
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return Config{}, strings.Join(loadedFrom, ","), err
	}
	return cfg, strings.Join(loadedFrom, ","), nil
}

func candidatePaths(explicitPath string, cwd string) []string {
	if explicitPath != "" {
		return []string{expandPath(explicitPath)}
	}
	home, _ := os.UserHomeDir()
	paths := []string{expandPath(filepath.Join(home, ".factory", "hooks", "lsp", "lspd.yaml"))}
	dir := cwd
	for dir != "" && dir != "/" {
		override := expandPath(filepath.Join(dir, ".factory", "lsp", "lspd.yaml"))
		if _, err := os.Stat(override); err == nil {
			paths = append(paths, override)
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return paths
}

func configNode(cfg Config) (*yaml.Node, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode default config: %w", err)
	}
	return decodeNode(data)
}

func decodeNode(data []byte) (*yaml.Node, error) {
	var node yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	return &node, nil
}

func mergeNodes(base, overlay *yaml.Node) {
	if base == nil || overlay == nil {
		return
	}
	if base.Kind == yaml.DocumentNode {
		if len(base.Content) == 0 {
			base.Content = []*yaml.Node{cloneNode(overlay)}
			return
		}
		if overlay.Kind == yaml.DocumentNode && len(overlay.Content) > 0 {
			mergeNodes(base.Content[0], overlay.Content[0])
			return
		}
		mergeNodes(base.Content[0], overlay)
		return
	}
	if overlay.Kind == yaml.DocumentNode {
		if len(overlay.Content) == 0 {
			return
		}
		mergeNodes(base, overlay.Content[0])
		return
	}
	if base.Kind == yaml.MappingNode && overlay.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(overlay.Content); i += 2 {
			key := overlay.Content[i]
			value := overlay.Content[i+1]
			index := mappingIndex(base, key.Value)
			if index >= 0 {
				mergeNodes(base.Content[index+1], value)
				continue
			}
			base.Content = append(base.Content, cloneNode(key), cloneNode(value))
		}
		return
	}
	*base = *cloneNode(overlay)
}

func mappingIndex(node *yaml.Node, key string) int {
	if node == nil || node.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func cloneNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			cloned.Content[i] = cloneNode(child)
		}
	}
	return &cloned
}
