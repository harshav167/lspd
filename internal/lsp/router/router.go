package router

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/client"
	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/lsp/supervisor"
	"github.com/harshav167/lspd/internal/metrics"
)

// Router resolves file paths to language server managers.
type Router struct {
	cfg      config.Config
	store    *store.Store
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	metrics  *metrics.Registry
	mu       sync.Mutex
	managers map[string]*client.Manager
	supers   map[string]*supervisor.Supervisor
}

// New creates a router.
func New(cfg config.Config, diagnosticStore *store.Store, logger *slog.Logger, metricsRegistry *metrics.Registry) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	return &Router{
		cfg:      cfg,
		store:    diagnosticStore,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		metrics:  metricsRegistry,
		managers: map[string]*client.Manager{},
		supers:   map[string]*supervisor.Supervisor{},
	}
}

// UpdateConfig refreshes the router's view of language mappings and definitions.
func (r *Router) UpdateConfig(cfg config.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
}

// Resolve returns a manager for the provided path, creating it lazily.
func (r *Router) Resolve(ctx context.Context, path string) (*client.Manager, config.LanguageConfig, error) {
	ext := filepath.Ext(path)
	languageName, ok := r.cfg.LanguageByExt[ext]
	if !ok {
		return nil, config.LanguageConfig{}, fmt.Errorf("unsupported extension %s", ext)
	}
	lang := r.cfg.Languages[languageName]
	root := detectRoot(path, lang.RootMarkers)
	key := languageName + ":" + root

	r.mu.Lock()
	defer r.mu.Unlock()
	if manager, ok := r.managers[key]; ok {
		return manager, lang, nil
	}
	manager, err := client.NewManager(ctx, lang, root, r.store, r.logger, r.metrics)
	if err != nil {
		return nil, config.LanguageConfig{}, err
	}
	r.managers[key] = manager
	super := supervisor.New(lang, root, r.store, r.logger, r.metrics)
	r.supers[key] = super
	go func() {
		_, _ = super.Run(r.ctx, manager, func(replacement *client.Manager) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.managers[key] = replacement
		})
	}()
	return manager, lang, nil
}

// Snapshot returns the known managers.
func (r *Router) Snapshot() map[string]*client.Manager {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*client.Manager, len(r.managers))
	for key, manager := range r.managers {
		out[key] = manager
	}
	return out
}

// ManagerState is a status-friendly snapshot of a running manager.
type ManagerState struct {
	Key        string    `json:"key"`
	Language   string    `json:"language"`
	Root       string    `json:"root"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	Documents  []string  `json:"documents"`
	Supervisor string    `json:"supervisor_state"`
}

// States returns sorted manager status entries.
func (r *Router) States() []ManagerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.managers))
	for key := range r.managers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	states := make([]ManagerState, 0, len(keys))
	for _, key := range keys {
		manager := r.managers[key]
		superState := string(supervisor.StateHealthy)
		if super, ok := r.supers[key]; ok {
			superState = string(super.State())
		}
		states = append(states, ManagerState{
			Key:        key,
			Language:   manager.Language(),
			Root:       manager.Root(),
			PID:        manager.PID(),
			StartedAt:  manager.StartedAt(),
			Documents:  manager.DocumentPaths(),
			Supervisor: superState,
		})
	}
	return states
}

// Close shuts down supervisors and all running managers.
func (r *Router) Close(ctx context.Context) error {
	r.cancel()

	r.mu.Lock()
	managers := make([]*client.Manager, 0, len(r.managers))
	for _, manager := range r.managers {
		managers = append(managers, manager)
	}
	r.managers = map[string]*client.Manager{}
	r.supers = map[string]*supervisor.Supervisor{}
	r.mu.Unlock()

	var firstErr error
	for _, manager := range managers {
		if err := manager.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
