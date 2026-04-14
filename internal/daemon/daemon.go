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

	"github.com/harshav167/lspd/internal/config"
	daemonlog "github.com/harshav167/lspd/internal/log"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	internalmcp "github.com/harshav167/lspd/internal/mcp"
	"github.com/harshav167/lspd/internal/metrics"
	"github.com/harshav167/lspd/internal/policy"
	"github.com/harshav167/lspd/internal/socket"
	"github.com/harshav167/lspd/internal/watcher"
	"go.lsp.dev/protocol"
)

// App is the running lspd daemon.
type App struct {
	ConfigManager *config.Manager
	Config        config.Config
	Logger        *slog.Logger
	Store         *store.Store
	Policy        *policy.Engine
	Diagnostics   *policy.DiagnosticsService
	Router        *router.Router
	MCP           *internalmcp.Server
	Socket        *socket.Server
	Metrics       *metrics.Registry
	Watcher       *watcher.Watcher

	lock        *lockFile
	port        int
	startedAt   time.Time
	metricsSrv  *http.Server
	cancel      context.CancelFunc
	closeOnce   sync.Once
	reloadMu    sync.RWMutex
	ideLockPath string
	lastReload  *config.ReloadReport
}

// New creates the daemon app.
func New(configPath, cwd string) (*App, error) {
	manager, err := config.NewManager(configPath, cwd)
	if err != nil {
		return nil, err
	}
	cfg := manager.Current()
	logger := daemonlog.New(cfg)
	diagnosticStore := store.New()
	app := &App{
		ConfigManager: manager,
		Config:        cfg,
		Logger:        logger,
		Store:         diagnosticStore,
		Metrics:       metrics.New(),
		startedAt:     time.Now(),
	}
	var appRouter *router.Router
	app.Policy = policy.New(cfg.Policy, func(ctx context.Context, path string, diagnostic protocol.Diagnostic) ([]string, error) {
		if appRouter == nil {
			return nil, nil
		}
		manager, _, err := appRouter.Resolve(ctx, path)
		if err != nil {
			return nil, err
		}
		doc, err := manager.EnsureOpen(ctx, path)
		if err != nil {
			return nil, err
		}
		actions, err := manager.CodeAction(ctx, &protocol.CodeActionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: doc.URI},
			Range: protocol.Range{
				Start: protocol.Position{Line: diagnostic.Range.Start.Line, Character: 0},
				End:   protocol.Position{Line: diagnostic.Range.End.Line, Character: diagnostic.Range.End.Character + 256},
			},
			Context: protocol.CodeActionContext{
				Diagnostics: []protocol.Diagnostic{diagnostic},
				Only:        []protocol.CodeActionKind{protocol.CodeActionKind("quickfix")},
			},
		})
		if err != nil {
			return nil, err
		}
		titles := make([]string, 0, len(actions))
		for _, action := range actions {
			if action.Title != "" {
				titles = append(titles, action.Title)
			}
		}
		return titles, nil
	})
	app.Router = router.New(cfg, diagnosticStore, logger, app.Metrics)
	appRouter = app.Router
	app.Diagnostics = policy.NewDiagnosticsService(app.Router, diagnosticStore, app.Policy)
	app.MCP = internalmcp.NewServer(cfg, internalmcp.Dependencies{
		Config:  manager,
		Router:  app.Router,
		Store:   diagnosticStore,
		Policy:  app.Policy,
		Logger:  logger,
		Metrics: app.Metrics,
	})
	app.Socket = socket.NewServer(cfg.Socket.Path, diagnosticStore, socket.Callbacks{
		Peek:          app.peekDiagnostics,
		Drain:         app.drainDiagnostics,
		Forget:        func(request socket.Request) { app.Diagnostics.ResetSession(request.SessionID) },
		Status:        app.Status,
		Reload:        app.Reload,
		RecordRequest: app.Metrics.RecordRequest,
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
	started := false
	defer func() {
		if !started {
			_ = a.Close(context.Background())
		}
	}()

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
	if a.Watcher != nil {
		a.Watcher.Run(ctx)
		go a.Watcher.SyncRoots(ctx, a.watcherRoots)
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
	// Write IDE lock file so Droid auto-discovers lspd the same way it
	// discovers VS Code / Cursor / Windsurf via ~/.factory/ide/<port>.lock.
	// Non-fatal: if this fails, the wrapper/env-var path still works.
	if err := a.writeIdeLockFile(); err != nil {
		a.Logger.Warn("failed to write IDE lock file", "error", err)
	}

	started = true
	return nil
}

// writeIdeLockFile writes a Droid-compatible IDE lock file so that
// IdeContextManager.findMatchingIdeInstance auto-discovers lspd without
// needing FACTORY_VSCODE_MCP_PORT or a launcher wrapper.
//
// Lock file format (matches Droid's IdeLockFileData):
//
//	~/.factory/ide/<port>.lock
//	{"pid": N, "ideName": "lspd", "workspaceFolders": ["/Users/harsha"]}
//
// Uses the user's home directory as the workspace root so the lock file
// matches any cwd under ~/. Using "/" does NOT work because Droid's
// matchesWorkspace does cwd.startsWith(folder + path.sep) and
// "/" + "/" = "//" which no path starts with. Home dir works because
// "/users/harsha/" is a valid prefix for any project path.
//
// Droid's findMatchingIdeInstance prefers a real IDE (Cursor/VS Code)
// when running inside that IDE's terminal, so lspd only wins when no
// IDE terminal is detected — which is exactly the headless case
// lspd exists for.
func (a *App) writeIdeLockFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".factory", "ide")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{
		"pid":              os.Getpid(),
		"ideName":          "lspd",
		"workspaceFolders": []string{home},
	})
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, fmt.Sprintf("%d.lock", a.port))
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		return err
	}
	a.ideLockPath = lockPath
	return nil
}

// removeIdeLockFile cleans up the IDE lock file on shutdown.
func (a *App) removeIdeLockFile() {
	if a.ideLockPath != "" {
		_ = os.Remove(a.ideLockPath)
		a.ideLockPath = ""
	}
}

// SetCancel allows the caller to provide a process-level cancellation hook.
func (a *App) SetCancel(cancel context.CancelFunc) {
	a.cancel = cancel
}

// Reload reloads daemon configuration and reapplies fields that can change at runtime.
func (a *App) Reload(ctx context.Context) error {
	cfg, report, err := a.ConfigManager.Reload(ctx)
	if err != nil {
		if a.Logger != nil {
			a.Logger.Error("config reload failed", "error", err)
		}
		return err
	}
	a.reloadMu.Lock()
	a.lastReload = &report
	a.reloadMu.Unlock()
	a.Config = cfg
	if report.Changed {
		a.Policy.UpdateConfig(cfg.Policy)
		if a.Router != nil {
			a.Router.UpdateConfig(cfg)
		}
	}
	if a.Logger != nil {
		metadata := a.ConfigManager.Metadata()
		a.Logger.Info(
			"config reloaded",
			"message", report.Message(),
			"changed", report.Changed,
			"applied_now", report.AppliedNow,
			"deferred", report.Deferred,
			"generation", metadata.Generation,
			"loaded_paths", metadata.LoadedPaths,
		)
	}
	return nil
}

// Close stops the daemon services.
func (a *App) Close(ctx context.Context) error {
	var closeErr error
	a.closeOnce.Do(func() {
		a.removeIdeLockFile()
		if a.cancel != nil {
			a.cancel()
			a.cancel = nil
		}
		if a.metricsSrv != nil {
			_ = a.metricsSrv.Shutdown(ctx)
		}
		if a.Socket != nil {
			_ = a.Socket.Close()
		}
		if a.MCP != nil {
			_ = a.MCP.Close(ctx)
		}
		if a.Router != nil {
			_ = a.Router.Close(ctx)
		}
		if a.Config.RunDir != "" {
			_ = os.Remove(filepath.Join(a.Config.RunDir, "lspd.port"))
		}
		if a.lock != nil {
			closeErr = a.lock.close()
			a.lock = nil
		}
	})
	return closeErr
}

// Port returns the MCP port.
func (a *App) Port() int {
	return a.port
}

// Status returns a basic daemon snapshot.
func (a *App) Status() map[string]any {
	now := time.Now()
	states := a.Router.States()
	entries := a.Store.Snapshot()
	type languageSummary struct {
		Diagnostics int       `json:"diagnostics"`
		EntryCount  int       `json:"entry_count"`
		LastPublish time.Time `json:"last_publish"`
	}
	languageSummaries := map[string]languageSummary{}
	totalDiagnostics := 0
	for _, entry := range entries {
		totalDiagnostics += len(entry.Diagnostics)
		summary := languageSummaries[entry.Language]
		summary.Diagnostics += len(entry.Diagnostics)
		summary.EntryCount++
		if entry.UpdatedAt.After(summary.LastPublish) {
			summary.LastPublish = entry.UpdatedAt
		}
		languageSummaries[entry.Language] = summary
	}
	languageStates := make([]map[string]any, 0, len(states))
	for _, state := range states {
		a.Metrics.SetOpenDocuments(state.Language, len(state.Documents))
		summary := languageSummaries[state.Language]
		languageStates = append(languageStates, map[string]any{
			"key":              state.Key,
			"language":         state.Language,
			"root":             state.Root,
			"pid":              state.PID,
			"started_at":       state.StartedAt,
			"uptime":           now.Sub(state.StartedAt).String(),
			"documents":        state.Documents,
			"document_count":   len(state.Documents),
			"supervisor_state": state.Supervisor,
			"last_publish":     summary.LastPublish,
			"diagnostics":      summary.Diagnostics,
		})
	}
	metadata := a.ConfigManager.Metadata()
	reloadStatus := map[string]any{
		"requested": false,
		"changed":   false,
		"message":   "startup config only",
	}
	a.reloadMu.RLock()
	lastReload := a.lastReload
	a.reloadMu.RUnlock()
	if lastReload != nil {
		reloadStatus = map[string]any{
			"requested":    true,
			"changed":      lastReload.Changed,
			"applied_now":  append([]string(nil), lastReload.AppliedNow...),
			"deferred":     append([]string(nil), lastReload.Deferred...),
			"message":      lastReload.Message(),
		}
	}
	metricsStatus := map[string]any{
		"enabled": false,
	}
	if a.Config.Metrics.Enabled {
		metricsURL := fmt.Sprintf("http://%s:%d/metrics", a.Config.Metrics.Host, a.Config.Metrics.Port)
		metricsStatus = map[string]any{
			"enabled":   true,
			"url":       metricsURL,
			"debug_url": "",
		}
		if a.Config.Metrics.Debug {
			metricsStatus["debug_url"] = fmt.Sprintf("http://%s:%d/debug/lspd", a.Config.Metrics.Host, a.Config.Metrics.Port)
		}
	}
	return map[string]any{
		"version":            "0.1.0",
		"pid":                os.Getpid(),
		"port":               a.port,
		"mcp_url":            fmt.Sprintf("http://%s:%d%s", a.Config.MCP.Host, a.port, a.Config.MCP.Endpoint),
		"started_at":         a.startedAt,
		"uptime":             now.Sub(a.startedAt).String(),
		"idle":               "disabled",
		"idle_timeout":       "disabled",
		"socket_path":        a.Config.Socket.Path,
		"log_file":           a.Config.LogFile,
		"log_level":          a.Config.LogLevel,
		"log_format":         a.Config.LogFormat,
		"config_path":        metadata.LoadedFrom,
		"config_paths":       metadata.LoadedPaths,
		"config_generation":  metadata.Generation,
		"config_reloaded_at": metadata.ReloadedAt,
		"entries":            entries,
		"language_states":    languageStates,
		"language_stats":     languageSummaries,
		"diagnostic_store": map[string]any{
			"entries":           len(entries),
			"total_diagnostics": totalDiagnostics,
		},
		"policy": map[string]any{
			"minimum_severity":                a.Config.Policy.MinimumSeverity,
			"max_per_file":                    a.Config.Policy.MaxPerFile,
			"max_per_turn":                    a.Config.Policy.MaxPerTurn,
			"attach_code_actions":             a.Config.Policy.AttachCodeActions,
			"max_code_actions_per_diagnostic": a.Config.Policy.MaxCodeActionsPerDiagnostic,
		},
		"reload":         reloadStatus,
		"metrics":        metricsStatus,
		"session_header": a.Config.MCP.SessionHeader,
	}
}

func (a *App) peekDiagnostics(ctx context.Context, request socket.Request) (store.Entry, map[string][]string, bool, error) {
	return a.fetchDiagnostics(ctx, request)
}

func (a *App) drainDiagnostics(ctx context.Context, request socket.Request) (store.Entry, map[string][]string, bool, error) {
	return a.fetchDiagnostics(ctx, request)
}

func (a *App) fetchDiagnostics(ctx context.Context, request socket.Request) (store.Entry, map[string][]string, bool, error) {
	if a.Diagnostics == nil {
		return store.Entry{}, nil, false, nil
	}
	result, err := a.Diagnostics.Fetch(ctx, policy.DiagnosticsRequest{
		Path:         request.Path,
		URI:          request.URI,
		SessionID:    request.SessionID,
		Freshness:    request.DiagnosticsFreshness(),
		Presentation: request.DiagnosticsPresentation(),
	})
	if err != nil {
		return store.Entry{}, nil, false, err
	}
	if !result.Found {
		return store.Entry{}, nil, false, nil
	}
	return result.Entry, result.CodeActions, true, nil
}

func (a *App) handleWatchedFile(ctx context.Context, path string) error {
	manager, _, err := a.Router.Resolve(ctx, path)
	if err != nil {
		return nil
	}
	_, err = manager.EnsureOpen(ctx, path)
	return err
}

func (a *App) watcherRoots() []string {
	states := a.Router.States()
	roots := make([]string, 0, len(states))
	for _, state := range states {
		roots = append(roots, state.Root)
	}
	return roots
}

func (a *App) enforceIdle(ctx context.Context) {
	_ = ctx
}
