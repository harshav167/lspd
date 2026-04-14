package router

import (
	"context"
	"log/slog"
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
	sessions map[string]*session
}

type session struct {
	lang    config.LanguageConfig
	root    string
	super   *supervisor.Supervisor
	mu      sync.RWMutex
	manager *client.Manager
}

func newSession(lang config.LanguageConfig, root string, manager *client.Manager, super *supervisor.Supervisor) *session {
	return &session{
		lang:    lang,
		root:    root,
		super:   super,
		manager: manager,
	}
}

func (s *session) Manager() *client.Manager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manager
}

func (s *session) Replace(manager *client.Manager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manager = manager
}

func (s *session) ResolveDocument(ctx context.Context, path string) (*client.Manager, client.Document, error) {
	manager := s.Manager()
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		return nil, client.Document{}, err
	}
	return manager, doc, nil
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
		sessions: map[string]*session{},
	}
}

// UpdateConfig refreshes the router's view of language mappings and definitions.
func (r *Router) UpdateConfig(cfg config.Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg = cfg
}

func (r *Router) sessionFor(ctx context.Context, path string) (*session, route, error) {
	route, err := resolveRoute(r.cfg, path)
	if err != nil {
		return nil, route, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.sessions[route.key]; ok {
		return existing, route, nil
	}

	manager, err := client.NewManager(ctx, route.lang, route.root, r.store, r.logger, r.metrics)
	if err != nil {
		return nil, route, err
	}
	super := supervisor.New(route.lang, route.root, r.store, r.logger, r.metrics)
	session := newSession(route.lang, route.root, manager, super)
	r.sessions[route.key] = session
	go func() {
		_, _ = super.Run(r.ctx, manager, session.Replace)
	}()
	return session, route, nil
}

// Resolve returns a manager for the provided path, creating it lazily.
func (r *Router) Resolve(ctx context.Context, path string) (*client.Manager, config.LanguageConfig, error) {
	session, route, err := r.sessionFor(ctx, path)
	if err != nil {
		return nil, config.LanguageConfig{}, err
	}
	return session.Manager(), route.lang, nil
}

// ResolveDocument returns a manager and guarantees the target document is registered.
func (r *Router) ResolveDocument(ctx context.Context, path string) (*client.Manager, client.Document, config.LanguageConfig, error) {
	session, route, err := r.sessionFor(ctx, path)
	if err != nil {
		return nil, client.Document{}, config.LanguageConfig{}, err
	}
	manager, doc, err := session.ResolveDocument(ctx, path)
	if err != nil {
		return nil, client.Document{}, config.LanguageConfig{}, err
	}
	return manager, doc, route.lang, nil
}

// Snapshot returns the known managers.
func (r *Router) Snapshot() map[string]*client.Manager {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*client.Manager, len(r.sessions))
	for key, session := range r.sessions {
		out[key] = session.Manager()
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
	keys := make([]string, 0, len(r.sessions))
	for key := range r.sessions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	states := make([]ManagerState, 0, len(keys))
	for _, key := range keys {
		session := r.sessions[key]
		manager := session.Manager()
		superState := string(supervisor.StateHealthy)
		if session.super != nil {
			superState = string(session.super.State())
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
	managers := make([]*client.Manager, 0, len(r.sessions))
	for _, session := range r.sessions {
		managers = append(managers, session.Manager())
	}
	r.sessions = map[string]*session{}
	r.mu.Unlock()

	var firstErr error
	for _, manager := range managers {
		if err := manager.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
