package config

import (
	"context"
	"reflect"
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

// Reload reloads the config from disk.
func (m *Manager) Reload(_ context.Context) (Config, bool, error) {
	cfg, loadedFrom, err := Load(m.path, m.cwd)
	if err != nil {
		return Config{}, false, err
	}
	current := m.currentState()
	updated := state{
		Config:      cfg,
		LoadedFrom:  loadedFrom,
		LoadedPaths: splitLoadedPaths(loadedFrom),
		Generation:  current.Generation,
		ReloadedAt:  current.ReloadedAt,
	}
	changed := !reflect.DeepEqual(current.Config, cfg) || current.LoadedFrom != loadedFrom
	if changed {
		updated.Generation++
		updated.ReloadedAt = time.Now()
	}
	m.value.Store(updated)
	if !changed {
		return cfg, false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subs {
		select {
		case ch <- cfg:
		default:
		}
	}
	return cfg, true, nil
}

func (m *Manager) currentState() state {
	return m.value.Load().(state)
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
