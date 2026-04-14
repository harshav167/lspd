package config

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type state struct {
	Config      Config
	LoadedFrom  string
	LoadedPaths []string
	Generation  uint64
	ReloadedAt  time.Time
}

// Metadata describes the currently loaded configuration.
type Metadata struct {
	LoadedFrom  string    `json:"loaded_from"`
	LoadedPaths []string  `json:"loaded_paths"`
	Generation  uint64    `json:"generation"`
	ReloadedAt  time.Time `json:"reloaded_at"`
}

// ReloadReport describes what a runtime reload changed immediately vs what stays deferred.
type ReloadReport struct {
	Changed    bool     `json:"changed"`
	AppliedNow []string `json:"applied_now,omitempty"`
	Deferred   []string `json:"deferred,omitempty"`
}

// Message summarizes the reload truth in one line.
func (r ReloadReport) Message() string {
	switch {
	case !r.Changed:
		return "config unchanged"
	case len(r.AppliedNow) == 0 && len(r.Deferred) == 0:
		return "config reloaded"
	case len(r.Deferred) == 0:
		return fmt.Sprintf("reload applied now: %s", strings.Join(r.AppliedNow, "; "))
	case len(r.AppliedNow) == 0:
		return fmt.Sprintf("reload accepted but deferred until restart: %s", strings.Join(r.Deferred, "; "))
	default:
		return fmt.Sprintf(
			"reload applied now: %s; deferred until restart: %s",
			strings.Join(r.AppliedNow, "; "),
			strings.Join(r.Deferred, "; "),
		)
	}
}

// Manager stores the current config and broadcasts reloads.
type Manager struct {
	path  string
	cwd   string
	value atomic.Value
	mu    sync.Mutex
	subs  []chan Config
}

// NewManager creates a config manager.
func NewManager(path, cwd string) (*Manager, error) {
	cfg, loadedFrom, err := Load(path, cwd)
	if err != nil {
		return nil, err
	}
	m := &Manager{path: path, cwd: cwd}
	m.value.Store(state{
		Config:      cfg,
		LoadedFrom:  loadedFrom,
		LoadedPaths: splitLoadedPaths(loadedFrom),
		Generation:  1,
		ReloadedAt:  time.Now(),
	})
	return m, nil
}

// Current returns the current config.
func (m *Manager) Current() Config {
	return m.currentState().Config
}

// Metadata returns details about the current configuration source and version.
func (m *Manager) Metadata() Metadata {
	current := m.currentState()
	return Metadata{
		LoadedFrom:  current.LoadedFrom,
		LoadedPaths: append([]string(nil), current.LoadedPaths...),
		Generation:  current.Generation,
		ReloadedAt:  current.ReloadedAt,
	}
}

// Subscribe registers a subscriber channel for reloads.
func (m *Manager) Subscribe() <-chan Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan Config, 1)
	m.subs = append(m.subs, ch)
	return ch
}

// Reload reloads the config from disk and classifies what changed immediately vs later.
func (m *Manager) Reload(_ context.Context) (Config, ReloadReport, error) {
	cfg, loadedFrom, err := Load(m.path, m.cwd)
	if err != nil {
		return Config{}, ReloadReport{}, err
	}
	current := m.currentState()
	report := classifyReload(current.Config, cfg, current.LoadedFrom, loadedFrom)
	updated := state{
		Config:      cfg,
		LoadedFrom:  loadedFrom,
		LoadedPaths: splitLoadedPaths(loadedFrom),
		Generation:  current.Generation,
		ReloadedAt:  current.ReloadedAt,
	}
	if report.Changed {
		updated.Generation++
		updated.ReloadedAt = time.Now()
	}
	m.value.Store(updated)
	if !report.Changed {
		return cfg, report, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subs {
		select {
		case ch <- cfg:
		default:
		}
	}
	return cfg, report, nil
}

func (m *Manager) currentState() state {
	return m.value.Load().(state)
}

func classifyReload(current, next Config, currentLoadedFrom, nextLoadedFrom string) ReloadReport {
	report := ReloadReport{
		Changed: !reflect.DeepEqual(current, next) || currentLoadedFrom != nextLoadedFrom,
	}
	if !report.Changed {
		return report
	}
	if currentLoadedFrom != nextLoadedFrom {
		report.AppliedNow = appendUnique(report.AppliedNow, "config source metadata")
	}
	if !reflect.DeepEqual(current.Policy, next.Policy) {
		report.AppliedNow = appendUnique(report.AppliedNow, "diagnostic policy")
	}
	if !reflect.DeepEqual(current.LanguageByExt, next.LanguageByExt) || !reflect.DeepEqual(current.Languages, next.Languages) {
		report.AppliedNow = appendUnique(report.AppliedNow, "language routing for newly resolved files")
		report.Deferred = appendUnique(report.Deferred, "already-running language server sessions keep prior command/settings until restart")
	}
	if current.RunDir != next.RunDir || current.IdleTimeout != next.IdleTimeout {
		report.Deferred = appendUnique(report.Deferred, "runtime directories and idle policy")
	}
	if current.Debug != next.Debug ||
		current.LogFile != next.LogFile ||
		current.LogLevel != next.LogLevel ||
		current.LogFormat != next.LogFormat ||
		current.LogMaxSizeMB != next.LogMaxSizeMB ||
		current.LogMaxBackups != next.LogMaxBackups ||
		current.LogMaxAgeDays != next.LogMaxAgeDays {
		report.Deferred = appendUnique(report.Deferred, "logger output settings")
	}
	if !reflect.DeepEqual(current.MCP, next.MCP) {
		report.Deferred = appendUnique(report.Deferred, "MCP listener settings")
	}
	if !reflect.DeepEqual(current.Socket, next.Socket) {
		report.Deferred = appendUnique(report.Deferred, "socket listener path")
	}
	if !reflect.DeepEqual(current.Metrics, next.Metrics) {
		report.Deferred = appendUnique(report.Deferred, "metrics listener settings")
	}
	if !reflect.DeepEqual(current.Watcher, next.Watcher) {
		report.Deferred = appendUnique(report.Deferred, "watcher lifecycle and debounce settings")
	}
	slices.Sort(report.AppliedNow)
	slices.Sort(report.Deferred)
	return report
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func splitLoadedPaths(loadedFrom string) []string {
	if loadedFrom == "" {
		return nil
	}
	parts := strings.Split(loadedFrom, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
