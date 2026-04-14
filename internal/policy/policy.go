package policy

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	"go.lsp.dev/protocol"
)

// CodeActionFetcher fetches code action titles for a diagnostic.
type CodeActionFetcher func(context.Context, string, protocol.Diagnostic) ([]string, error)

// Engine applies diagnostic surfacing rules.
type Engine struct {
	mu      sync.Mutex
	seen    map[string]map[string]struct{}
	cfg     config.PolicyConfig
	fetcher CodeActionFetcher
}

// Result is the filtered diagnostics plus quick-fix previews.
type Result struct {
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
	CodeActions map[string][]string   `json:"code_actions,omitempty"`
}

// DiagnosticsFreshness controls how aggressively diagnostics are refreshed.
type DiagnosticsFreshness string

const (
	DiagnosticsFreshnessPeek          DiagnosticsFreshness = "peek"
	DiagnosticsFreshnessDrain         DiagnosticsFreshness = "drain"
	DiagnosticsFreshnessBestEffortNow DiagnosticsFreshness = "best-effort-now"
)

// DiagnosticsPresentation controls whether callers want raw or surfaced diagnostics.
type DiagnosticsPresentation string

const (
	DiagnosticsPresentationRaw      DiagnosticsPresentation = "raw"
	DiagnosticsPresentationSurfaced DiagnosticsPresentation = "surfaced"
)

// DiagnosticsRequest captures the diagnostics-domain semantics independent of transport.
type DiagnosticsRequest struct {
	Path         string
	URI          string
	SessionID    string
	Freshness    DiagnosticsFreshness
	Presentation DiagnosticsPresentation
}

// DiagnosticsResult is the transport-agnostic diagnostics fetch result.
type DiagnosticsResult struct {
	Entry       store.Entry
	CodeActions map[string][]string
	Found       bool
}

// DiagnosticsService coordinates diagnostics retrieval and presentation across transports.
type DiagnosticsService struct {
	router      *router.Router
	store       *store.Store
	policy      *Engine
	waitTimeout time.Duration
}

// New creates a new policy engine.
func New(cfg config.PolicyConfig, fetcher CodeActionFetcher) *Engine {
	return &Engine{seen: map[string]map[string]struct{}{}, cfg: cfg, fetcher: fetcher}
}

// NewDiagnosticsService creates a diagnostics-domain service boundary.
func NewDiagnosticsService(router *router.Router, diagnosticStore *store.Store, engine *Engine) *DiagnosticsService {
	return &DiagnosticsService{
		router:      router,
		store:       diagnosticStore,
		policy:      engine,
		waitTimeout: 1200 * time.Millisecond,
	}
}

// UpdateConfig updates runtime config.
func (e *Engine) UpdateConfig(cfg config.PolicyConfig) { e.cfg = cfg }

// Apply applies policy filters and remembers emitted diagnostics per session.
func (e *Engine) Apply(ctx context.Context, sessionID string, path string, diagnostics []protocol.Diagnostic) Result {
	return e.Surface(ctx, sessionID, path, diagnostics)
}

// Surface applies the surfaced/read-path policy semantics.
func (e *Engine) Surface(ctx context.Context, sessionID string, path string, diagnostics []protocol.Diagnostic) Result {
	filtered := filterDiagnostics(e.cfg, diagnostics)
	filtered = e.dedup(sessionID, path, filtered)
	filtered = limitDiagnostics(filtered, e.cfg.MaxPerFile, e.cfg.MaxPerTurn)
	return Result{
		Diagnostics: filtered,
		CodeActions: e.codeActionsFor(ctx, path, filtered),
	}
}

// Fetch retrieves diagnostics according to the requested freshness and presentation semantics.
func (s *DiagnosticsService) Fetch(ctx context.Context, req DiagnosticsRequest) (DiagnosticsResult, error) {
	path, uri, err := normalizeDiagnosticsTarget(req)
	if err != nil {
		return DiagnosticsResult{}, err
	}

	cached, cachedOK := s.peek(uri)

	switch req.Freshness {
	case DiagnosticsFreshnessDrain:
		entry, ok, fetchErr := s.fetchDrain(ctx, path)
		if fetchErr != nil {
			return DiagnosticsResult{}, fetchErr
		}
		return s.present(ctx, req, path, entry, ok), nil
	case DiagnosticsFreshnessBestEffortNow:
		entry, ok, _ := s.fetchDrain(ctx, path)
		if ok {
			return s.present(ctx, req, path, entry, true), nil
		}
		return s.present(ctx, req, path, cached, cachedOK), nil
	case DiagnosticsFreshnessPeek, "":
		fallthrough
	default:
		return s.present(ctx, req, path, cached, cachedOK), nil
	}
}

// ResetSession clears remembered surfaced diagnostics for a session.
func (s *DiagnosticsService) ResetSession(sessionID string) {
	if s == nil || s.policy == nil {
		return
	}
	s.policy.ResetSession(sessionID)
}

func (s *DiagnosticsService) peek(uri protocol.DocumentURI) (store.Entry, bool) {
	if s == nil || s.store == nil {
		return store.Entry{}, false
	}
	return s.store.Peek(uri)
}

func (s *DiagnosticsService) fetchDrain(ctx context.Context, path string) (store.Entry, bool, error) {
	if s == nil || s.router == nil || s.store == nil {
		return store.Entry{}, false, nil
	}
	manager, _, err := s.router.Resolve(ctx, path)
	if err != nil {
		return store.Entry{}, false, err
	}
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		return store.Entry{}, false, err
	}
	entry, ok, waitErr := s.store.Wait(ctx, doc.URI, doc.Version, s.waitTimeout)
	if !ok {
		return store.Entry{}, false, waitErr
	}
	return entry, true, nil
}

func (s *DiagnosticsService) present(ctx context.Context, req DiagnosticsRequest, path string, entry store.Entry, ok bool) DiagnosticsResult {
	if !ok {
		return DiagnosticsResult{}
	}
	if req.Presentation == DiagnosticsPresentationSurfaced && s != nil && s.policy != nil {
		surfaced := s.policy.Surface(ctx, req.SessionID, path, entry.Diagnostics)
		return DiagnosticsResult{
			Entry:       entry.WithDiagnostics(surfaced.Diagnostics),
			CodeActions: surfaced.CodeActions,
			Found:       true,
		}
	}
	return DiagnosticsResult{Entry: entry, Found: true}
}

func normalizeDiagnosticsTarget(req DiagnosticsRequest) (string, protocol.DocumentURI, error) {
	if req.Path != "" {
		path := filepath.Clean(req.Path)
		return path, store.URIFromPath(path), nil
	}
	if req.URI == "" {
		return "", "", fmt.Errorf("diagnostics target is required")
	}
	parsed, err := url.Parse(req.URI)
	if err != nil {
		return "", "", err
	}
	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return "", "", fmt.Errorf("unsupported URI scheme %q for %q: only file:// URIs are supported", parsed.Scheme, req.URI)
	}
	path := filepath.Clean(parsed.Path)
	return path, protocol.DocumentURI(req.URI), nil
}
