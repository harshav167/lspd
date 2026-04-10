package config

import (
	"context"
	"sync"
	"sync/atomic"
)

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
	cfg, _, err := Load(path, cwd)
	if err != nil {
		return nil, err
	}
	m := &Manager{path: path, cwd: cwd}
	m.value.Store(cfg)
	return m, nil
}

// Current returns the current config.
func (m *Manager) Current() Config {
	return m.value.Load().(Config)
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
func (m *Manager) Reload(_ context.Context) (Config, error) {
	cfg, _, err := Load(m.path, m.cwd)
	if err != nil {
		return Config{}, err
	}
	m.value.Store(cfg)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subs {
		select {
		case ch <- cfg:
		default:
		}
	}
	return cfg, nil
}
