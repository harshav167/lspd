package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.lsp.dev/protocol"
)

// Duration wraps time.Duration for YAML-friendly parsing.
type Duration struct {
	time.Duration
}

// UnmarshalText parses a duration from text.
func (d *Duration) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", string(text), err)
	}
	d.Duration = parsed
	return nil
}

// MarshalText returns the duration as a string.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Config is the full daemon configuration.
type Config struct {
	RunDir        string                    `yaml:"run_dir"`
	LogFile       string                    `yaml:"log_file"`
	Debug         bool                      `yaml:"debug"`
	LogLevel      string                    `yaml:"log_level"`
	LogFormat     string                    `yaml:"log_format"`
	LogMaxSizeMB  int                       `yaml:"log_max_size_mb"`
	LogMaxBackups int                       `yaml:"log_max_backups"`
	LogMaxAgeDays int                       `yaml:"log_max_age_days"`
	IdleTimeout   Duration                  `yaml:"idle_timeout"`
	MCP           MCPConfig                 `yaml:"mcp"`
	Socket        SocketConfig              `yaml:"socket"`
	Metrics       MetricsConfig             `yaml:"metrics"`
	Policy        PolicyConfig              `yaml:"policy"`
	Watcher       WatcherConfig             `yaml:"watcher"`
	LanguageByExt map[string]string         `yaml:"language_by_ext"`
	Languages     map[string]LanguageConfig `yaml:"languages"`
}

// MCPConfig configures the MCP server.
type MCPConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	SessionHeader string `yaml:"session_header"`
	Endpoint      string `yaml:"endpoint"`
}

// SocketConfig configures the unix socket endpoint.
type SocketConfig struct {
	Path string `yaml:"path"`
}

// MetricsConfig configures the optional Prometheus endpoint.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Debug   bool   `yaml:"debug"`
}

// WatcherConfig configures the fsnotify bridge.
type WatcherConfig struct {
	Enabled  bool     `yaml:"enabled"`
	Debounce Duration `yaml:"debounce"`
}

// PolicyConfig defines surfacing rules for diagnostics.
type PolicyConfig struct {
	MaxPerFile                  int      `yaml:"max_per_file"`
	MaxPerTurn                  int      `yaml:"max_per_turn"`
	MinimumSeverity             int      `yaml:"minimum_severity"`
	AllowedSources              []string `yaml:"allowed_sources"`
	DeniedSources               []string `yaml:"denied_sources"`
	AttachCodeActions           bool     `yaml:"attach_code_actions"`
	MaxCodeActionsPerDiagnostic int      `yaml:"max_code_actions_per_diagnostic"`
}

// LanguageConfig defines how to start and speak to a language server.
type LanguageConfig struct {
	Name                  string                      `yaml:"name"`
	Command               string                      `yaml:"command"`
	Args                  []string                    `yaml:"args"`
	Env                   map[string]string           `yaml:"env"`
	Extensions            []string                    `yaml:"extensions"`
	RootMarkers           []string                    `yaml:"root_markers"`
	Settings              map[string]any              `yaml:"settings"`
	InitializationOptions map[string]any              `yaml:"initialization_options"`
	WorkspaceFolders      bool                        `yaml:"workspace_folders"`
	Warmup                bool                        `yaml:"warmup"`
	MaxRestarts           int                         `yaml:"max_restarts"`
	RestartWindow         Duration                    `yaml:"restart_window"`
	DocumentTTL           Duration                    `yaml:"document_ttl"`
	LanguageID            protocol.LanguageIdentifier `yaml:"language_id"`
}

// Default returns the default configuration.
func Default() Config {
	home, _ := os.UserHomeDir()
	runDir := filepath.Join(home, ".factory", "run")
	logFile := filepath.Join(home, ".factory", "logs", "lspd.log")
	socketPath := filepath.Join(runDir, "lspd.sock")
	return Config{
		RunDir:        runDir,
		LogFile:       logFile,
		Debug:         false,
		LogLevel:      "info",
		LogFormat:     "json",
		LogMaxSizeMB:  50,
		LogMaxBackups: 5,
		LogMaxAgeDays: 7,
		IdleTimeout:   Duration{Duration: 30 * time.Minute},
		MCP: MCPConfig{
			Host:          "127.0.0.1",
			Port:          0,
			SessionHeader: "X-Droid-Session-Id",
			Endpoint:      "/mcp",
		},
		Socket:  SocketConfig{Path: socketPath},
		Metrics: MetricsConfig{Enabled: false, Host: "127.0.0.1", Port: 39091},
		Watcher: WatcherConfig{Enabled: true, Debounce: Duration{Duration: 250 * time.Millisecond}},
		Policy:  PolicyConfig{MaxPerFile: 20, MaxPerTurn: 50, MinimumSeverity: 1, AttachCodeActions: true, MaxCodeActionsPerDiagnostic: 2},
		Languages: map[string]LanguageConfig{
			"ts": {
				Name:             "ts",
				Command:          "typescript-language-server",
				Args:             []string{"--stdio"},
				Extensions:       []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"},
				RootMarkers:      []string{"tsconfig.json", "package.json", ".git"},
				Settings:         map[string]any{},
				WorkspaceFolders: true,
				Warmup:           true,
				MaxRestarts:      5,
				RestartWindow:    Duration{Duration: 10 * time.Minute},
				DocumentTTL:      Duration{Duration: 15 * time.Minute},
				LanguageID:       protocol.LanguageIdentifier("typescript"),
			},
			"py": {
				Name:             "py",
				Command:          "pyright-langserver",
				Args:             []string{"--stdio"},
				Extensions:       []string{".py"},
				RootMarkers:      []string{"pyproject.toml", "setup.py", "requirements.txt", ".git"},
				Settings:         map[string]any{"python": map[string]any{"analysis": map[string]any{"typeCheckingMode": "basic"}}},
				WorkspaceFolders: true,
				Warmup:           true,
				MaxRestarts:      5,
				RestartWindow:    Duration{Duration: 10 * time.Minute},
				DocumentTTL:      Duration{Duration: 15 * time.Minute},
				LanguageID:       protocol.LanguageIdentifier("python"),
			},
			"go": {
				Name:             "go",
				Command:          "gopls",
				Extensions:       []string{".go", ".mod", ".sum"},
				RootMarkers:      []string{"go.mod", ".git"},
				Settings:         map[string]any{"gopls": map[string]any{"staticcheck": true}},
				WorkspaceFolders: true,
				Warmup:           true,
				MaxRestarts:      5,
				RestartWindow:    Duration{Duration: 10 * time.Minute},
				DocumentTTL:      Duration{Duration: 15 * time.Minute},
				LanguageID:       protocol.LanguageIdentifier("go"),
			},
			"rust": {
				Name:             "rust",
				Command:          "rust-analyzer",
				Extensions:       []string{".rs"},
				RootMarkers:      []string{"Cargo.toml", "rust-project.json", ".git"},
				WorkspaceFolders: true,
				Warmup:           false,
				MaxRestarts:      5,
				RestartWindow:    Duration{Duration: 10 * time.Minute},
				DocumentTTL:      Duration{Duration: 15 * time.Minute},
				LanguageID:       protocol.LanguageIdentifier("rust"),
			},
			"cpp": {
				Name:             "cpp",
				Command:          "clangd",
				Extensions:       []string{".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx"},
				RootMarkers:      []string{"compile_commands.json", ".clangd", ".git"},
				WorkspaceFolders: true,
				Warmup:           true,
				MaxRestarts:      5,
				RestartWindow:    Duration{Duration: 10 * time.Minute},
				DocumentTTL:      Duration{Duration: 15 * time.Minute},
				LanguageID:       protocol.LanguageIdentifier("cpp"),
			},
		},
		LanguageByExt: map[string]string{},
	}
}

// Normalize ensures the config contains derived defaults.
func (c *Config) Normalize() {
	c.RunDir = expandPath(c.RunDir)
	c.LogFile = expandPath(c.LogFile)
	c.Socket.Path = expandPath(c.Socket.Path)
	if c.MCP.Host == "" {
		c.MCP.Host = "127.0.0.1"
	}
	if c.MCP.Endpoint == "" {
		c.MCP.Endpoint = "/mcp"
	}
	if c.MCP.SessionHeader == "" {
		c.MCP.SessionHeader = "X-Droid-Session-Id"
	}
	if c.Watcher.Debounce.Duration <= 0 {
		c.Watcher.Debounce = Duration{Duration: 250 * time.Millisecond}
	}
	if c.IdleTimeout.Duration < 0 {
		c.IdleTimeout = Duration{Duration: 30 * time.Minute}
	}
	if c.LogLevel == "" {
		if c.Debug {
			c.LogLevel = "debug"
		} else {
			c.LogLevel = "info"
		}
	}
	if c.LogFormat == "" {
		c.LogFormat = "json"
	}
	if c.LogMaxSizeMB <= 0 {
		c.LogMaxSizeMB = 50
	}
	if c.LogMaxBackups <= 0 {
		c.LogMaxBackups = 5
	}
	if c.LogMaxAgeDays <= 0 {
		c.LogMaxAgeDays = 7
	}
	if c.Metrics.Host == "" {
		c.Metrics.Host = "127.0.0.1"
	}
	if c.Metrics.Port <= 0 {
		c.Metrics.Port = 39091
	}
	if c.Policy.MaxPerFile <= 0 {
		c.Policy.MaxPerFile = 20
	}
	if c.Policy.MaxPerTurn <= 0 {
		c.Policy.MaxPerTurn = 50
	}
	if c.Policy.MaxCodeActionsPerDiagnostic <= 0 {
		c.Policy.MaxCodeActionsPerDiagnostic = 2
	}
	if c.LanguageByExt == nil {
		c.LanguageByExt = map[string]string{}
	}
	for name, lang := range c.Languages {
		lang.Name = name
		if lang.RestartWindow.Duration <= 0 {
			lang.RestartWindow = Duration{Duration: 10 * time.Minute}
		}
		if lang.DocumentTTL.Duration <= 0 {
			lang.DocumentTTL = Duration{Duration: 15 * time.Minute}
		}
		if lang.MaxRestarts <= 0 {
			lang.MaxRestarts = 5
		}
		c.Languages[name] = lang
		for _, ext := range lang.Extensions {
			if ext == "" {
				continue
			}
			normalized := ext
			if !strings.HasPrefix(normalized, ".") {
				normalized = "." + normalized
			}
			c.LanguageByExt[normalized] = name
		}
	}
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	expanded := os.ExpandEnv(path)
	switch {
	case expanded == "~":
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home
		}
	case strings.HasPrefix(expanded, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	return filepath.Clean(expanded)
}
