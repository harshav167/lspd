package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/harsha/lspd/internal/config"
	daemonlog "github.com/harsha/lspd/internal/log"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	internalmcp "github.com/harsha/lspd/internal/mcp"
	"github.com/harsha/lspd/internal/metrics"
	"github.com/harsha/lspd/internal/policy"
	"github.com/harsha/lspd/internal/socket"
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

	lock       *lockFile
	port       int
	startedAt  time.Time
	metricsSrv *http.Server
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
	}
	app.Policy = policy.New(cfg.Policy, nil)
	app.Router = router.New(cfg, diagnosticStore, logger)
	app.MCP = internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config: manager,
		Router: app.Router,
		Store:  diagnosticStore,
		Policy: app.Policy,
		Logger: logger,
	})
	app.Socket = socket.NewServer(cfg.Socket.Path, diagnosticStore, app.Policy.ResetSession, func(ctx context.Context) error {
		_, err := manager.Reload(ctx)
		if err == nil {
			app.Config = manager.Current()
			app.Policy.UpdateConfig(app.Config.Policy)
		}
		return err
	}, app.Status)
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
	if a.Config.Metrics.Enabled {
		a.metricsSrv = &http.Server{Addr: fmt.Sprintf("%s:%d", a.Config.Metrics.Host, a.Config.Metrics.Port), Handler: a.Metrics.Handler()}
		go func() {
			_ = a.metricsSrv.ListenAndServe()
		}()
	}
	return nil
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
