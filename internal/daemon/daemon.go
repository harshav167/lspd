package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/harsha/lspd/internal/config"
	daemonlog "github.com/harsha/lspd/internal/log"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	internalmcp "github.com/harsha/lspd/internal/mcp"
	"github.com/harsha/lspd/internal/metrics"
	"github.com/harsha/lspd/internal/policy"
	"github.com/harsha/lspd/internal/socket"
	"github.com/harsha/lspd/internal/watcher"
	"go.lsp.dev/protocol"
)

// App is the running lspd daemon.
type App struct {
	ConfigManager *config.Manager
	Config        config.Config
	Logger        *slog.Logger
	Store         *store.Store
	Policy        *policy.Engine
	Router        *router.Router
	MCP           *internalmcp.Server
	Socket        *socket.Server
	Metrics       *metrics.Registry
	Watcher       *watcher.Watcher

	lock       *lockFile
	port       int
	startedAt  time.Time
	metricsSrv *http.Server
	mu         sync.Mutex
	watched    map[string]struct{}
	idle       *idleTimer
	cancel     context.CancelFunc
}

// New creates the daemon app.
func New(configPath, cwd string) (*App, error) {
	manager, err := config.NewManager(configPath, cwd)
	if err != nil {
		return nil, err
	}
	cfg := manager.Current()
	logger := daemonlog.New(cfg.LogFile, cfg.Debug)
	diagnosticStore := store.New()
	app := &App{
		ConfigManager: manager,
		Config:        cfg,
		Logger:        logger,
		Store:         diagnosticStore,
		Metrics:       metrics.New(),
		startedAt:     time.Now(),
		watched:       map[string]struct{}{},
		idle:          newIdleTimer(cfg.IdleTimeout.Duration),
	}
	app.Policy = policy.New(cfg.Policy, nil)
	app.Router = router.New(cfg, diagnosticStore, logger)
	app.MCP = internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: manager,
		Router: app.Router,
		Store:  diagnosticStore,
		Policy: app.Policy,
		Logger: logger,
		Touch:  app.Touch,
	})
	app.Socket = socket.NewServer(cfg.Socket.Path, diagnosticStore, socket.Callbacks{
		Peek:   app.peekDiagnostics,
		Drain:  app.drainDiagnostics,
		Forget: func(request socket.Request) { app.Policy.ResetSession(request.SessionID) },
		Status: app.Status,
		Reload: func(ctx context.Context) error {
			_, err := manager.Reload(ctx)
			if err == nil {
				app.Config = manager.Current()
				app.Policy.UpdateConfig(app.Config.Policy)
			}
			return err
		},
		Touch: app.Touch,
	})
	if cfg.Watcher.Enabled {
		fileWatcher, watcherErr := watcher.New(cfg.Watcher.Debounce.Duration, app.handleWatchedFile)
		if watcherErr != nil {
			return nil, watcherErr
		}
		app.Watcher = fileWatcher
	}
	return app, nil
}

// Start starts the daemon services.
func (a *App) Start(ctx context.Context) error {
	lockPath := filepath.Join(a.Config.RunDir, "lspd.pid")
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	a.lock = lockFile
	port, err := a.MCP.Start()
	if err != nil {
		return err
	}
	a.port = port
	if err := os.WriteFile(filepath.Join(a.Config.RunDir, "lspd.port"), []byte(fmt.Sprintf("%d", port)), 0o600); err != nil {
		return err
	}
	if err := a.Socket.Start(ctx); err != nil {
		return err
	}
	go a.enforceIdle(ctx)
	if a.Watcher != nil {
		a.Watcher.Run(ctx)
		go a.syncWatcherRoots(ctx)
	}
	if a.Config.Metrics.Enabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", a.Metrics.Handler())
		if a.Config.Metrics.Debug {
			mux.HandleFunc("/debug/lspd", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(a.Status())
			})
		}
		a.metricsSrv = &http.Server{Addr: fmt.Sprintf("%s:%d", a.Config.Metrics.Host, a.Config.Metrics.Port), Handler: mux}
		go func() {
			_ = a.metricsSrv.ListenAndServe()
		}()
	}
	return nil
}

// SetCancel allows the caller to provide a process-level cancellation hook.
func (a *App) SetCancel(cancel context.CancelFunc) {
	a.cancel = cancel
}

// Touch resets the idle timeout.
func (a *App) Touch() {
	if a.idle != nil {
		a.idle.touch()
	}
}

// Close stops the daemon services.
func (a *App) Close(ctx context.Context) error {
	if a.metricsSrv != nil {
		_ = a.metricsSrv.Shutdown(ctx)
	}
	if a.Socket != nil {
		_ = a.Socket.Close()
	}
	if a.MCP != nil {
		_ = a.MCP.Close(ctx)
	}
	if a.lock != nil {
		_ = a.lock.close()
	}
	return nil
}

// Port returns the MCP port.
func (a *App) Port() int {
	return a.port
}

// Status returns a basic daemon snapshot.
func (a *App) Status() map[string]any {
	return map[string]any{
		"port":            a.port,
		"started_at":      a.startedAt,
		"socket_path":     a.Config.Socket.Path,
		"entries":         a.Store.Snapshot(),
		"language_states": a.Router.States(),
		"metrics_enabled": a.Config.Metrics.Enabled,
		"idle_timeout":    a.Config.IdleTimeout.Duration.String(),
		"session_header":  a.Config.MCP.SessionHeader,
	}
}

func (a *App) peekDiagnostics(ctx context.Context, request socket.Request) (store.Entry, bool, error) {
	uri := protocol.DocumentURI("file://" + filepath.ToSlash(request.Path))
	entry, ok := a.Store.Peek(uri)
	if !ok {
		return store.Entry{}, false, nil
	}
	filtered := a.Policy.Apply(ctx, request.SessionID, string(uri), entry.Diagnostics)
	entry.Diagnostics = filtered.Diagnostics
	return entry, true, nil
}

func (a *App) drainDiagnostics(ctx context.Context, request socket.Request) (store.Entry, bool, error) {
	manager, _, err := a.Router.Resolve(ctx, request.Path)
	if err != nil {
		return a.peekDiagnostics(ctx, request)
	}
	doc, err := manager.EnsureOpen(ctx, request.Path)
	if err != nil {
		return store.Entry{}, false, err
	}
	entry, ok, waitErr := a.Store.Wait(ctx, doc.URI, doc.Version, 1200*time.Millisecond)
	if !ok {
		return store.Entry{}, false, waitErr
	}
	filtered := a.Policy.Apply(ctx, request.SessionID, string(doc.URI), entry.Diagnostics)
	entry.Diagnostics = filtered.Diagnostics
	return entry, true, nil
}

func (a *App) handleWatchedFile(ctx context.Context, path string) error {
	manager, _, err := a.Router.Resolve(ctx, path)
	if err != nil {
		return nil
	}
	_, err = manager.EnsureOpen(ctx, path)
	return err
}

func (a *App) syncWatcherRoots(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, state := range a.Router.States() {
				a.mu.Lock()
				_, seen := a.watched[state.Root]
				if !seen {
					a.watched[state.Root] = struct{}{}
				}
				a.mu.Unlock()
				if !seen {
					_ = a.Watcher.Add(state.Root)
				}
			}
		}
	}
}

func (a *App) enforceIdle(ctx context.Context) {
	if a.idle == nil || a.idle.timer == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-a.idle.timer.C:
		if a.cancel != nil {
			a.cancel()
			return
		}
		_ = a.Close(context.Background())
	}
}
