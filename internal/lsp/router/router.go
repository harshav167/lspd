package router

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/client"
	"github.com/harsha/lspd/internal/lsp/store"
)

// Router resolves file paths to language server managers.
type Router struct {
	cfg      config.Config
	store    *store.Store
	logger   *slog.Logger
	mu       sync.Mutex
	managers map[string]*client.Manager
}

// New creates a router.
func New(cfg config.Config, diagnosticStore *store.Store, logger *slog.Logger) *Router {
	return &Router{cfg: cfg, store: diagnosticStore, logger: logger, managers: map[string]*client.Manager{}}
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
	manager, err := client.NewManager(ctx, lang, root, r.store, r.logger)
	if err != nil {
		return nil, config.LanguageConfig{}, err
	}
	r.managers[key] = manager
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
	Key       string    `json:"key"`
	Language  string    `json:"language"`
	Root      string    `json:"root"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Documents []string  `json:"documents"`
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
		states = append(states, ManagerState{
			Key:       key,
			Language:  manager.Language(),
			Root:      manager.Root(),
			PID:       manager.PID(),
			StartedAt: manager.StartedAt(),
			Documents: manager.DocumentPaths(),
		})
	}
	return states
}
