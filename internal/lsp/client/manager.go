package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/lsp/store"
	"github.com/harshav167/lspd/internal/metrics"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.uber.org/zap"
)

type pipeReadWriteCloser struct {
	io.Reader
	io.Writer
	closers []io.Closer
}

func (p *pipeReadWriteCloser) Close() error {
	var first error
	for _, closer := range p.closers {
		if err := closer.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Manager owns a single language server subprocess and its tracked documents.
type Manager struct {
	cfg            config.LanguageConfig
	root           string
	store          *store.Store
	logger         *slog.Logger
	zapLogger      *zap.Logger
	cmd            *exec.Cmd
	conn           jsonrpc2.Conn
	server         protocol.Server
	tracker        *documentTracker
	metrics        *metrics.Registry
	mu             sync.RWMutex
	closed         bool
	requestTimeout time.Duration
	startedAt      time.Time
	runCtx         context.Context
	runCancel      context.CancelFunc
}

// NewManager starts and initializes a language server manager.
func NewManager(ctx context.Context, cfg config.LanguageConfig, root string, diagnosticStore *store.Store, logger *slog.Logger, metricsRegistry *metrics.Registry) (*Manager, error) {
	runCtx, runCancel := context.WithCancel(context.Background())
	manager := &Manager{
		cfg:            cfg,
		root:           root,
		store:          diagnosticStore,
		logger:         logger,
		zapLogger:      zap.NewNop(),
		tracker:        newDocumentTracker(),
		metrics:        metricsRegistry,
		requestTimeout: 5 * time.Second,
		startedAt:      time.Now(),
		runCtx:         runCtx,
		runCancel:      runCancel,
	}
	if err := manager.start(ctx); err != nil {
		runCancel()
		return nil, err
	}
	return manager, nil
}

// Root returns the manager workspace root.
func (m *Manager) Root() string {
	return m.root
}

// Wait blocks until the underlying jsonrpc connection is closed.
func (m *Manager) Wait() error {
	<-m.conn.Done()
	return m.conn.Err()
}

// Shutdown gracefully shuts the language server down.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	if m.runCancel != nil {
		m.runCancel()
	}
	if m.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = m.server.Shutdown(shutdownCtx)
		_ = m.server.Exit(shutdownCtx)
	}
	if m.conn != nil {
		_ = m.conn.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
	return nil
}

// TrackedDocs returns the currently tracked documents.
func (m *Manager) TrackedDocs() []trackedDocument {
	return m.tracker.list()
}

// Language returns the language name handled by this manager.
func (m *Manager) Language() string {
	return m.cfg.Name
}

// PID returns the subprocess pid if available.
func (m *Manager) PID() int {
	if m.cmd == nil || m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

// StartedAt returns the manager start time.
func (m *Manager) StartedAt() time.Time {
	return m.startedAt
}

// DocumentPaths returns sorted tracked document paths.
func (m *Manager) DocumentPaths() []string {
	docs := m.tracker.list()
	paths := make([]string, 0, len(docs))
	for _, doc := range docs {
		paths = append(paths, doc.Path)
	}
	sort.Strings(paths)
	return paths
}

func (m *Manager) start(ctx context.Context) error {
	stdin, stdout, stderr, cmd, err := m.spawn()
	if err != nil {
		return err
	}
	stream := jsonrpc2.NewStream(&pipeReadWriteCloser{
		Reader:  stdout,
		Writer:  stdin,
		closers: []io.Closer{stdin, stdout},
	})
	conn := jsonrpc2.NewConn(stream)
	conn.Go(m.runCtx, m.handleIncoming)

	m.cmd = cmd
	m.conn = conn
	m.server = protocol.ServerDispatcher(conn, m.zapLogger.Named(m.cfg.Name))

	go m.captureStderr(stderr)
	go m.reapIdleDocuments(m.runCtx)

	if err := m.initialize(ctx); err != nil {
		_ = m.Shutdown(context.Background())
		return err
	}
	return nil
}

func (m *Manager) spawn() (io.WriteCloser, io.ReadCloser, io.ReadCloser, *exec.Cmd, error) {
	cmd := exec.Command(m.cfg.Command, m.cfg.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Dir = m.root
	if len(m.cfg.Env) > 0 {
		env := make([]string, 0, len(m.cfg.Env))
		for key, value := range m.cfg.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("start %s: %w", m.cfg.Command, err)
	}
	return stdin, stdout, stderr, cmd, nil
}

func (m *Manager) captureStderr(stderr io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			m.logger.Debug("lsp stderr", "language", m.cfg.Name, "output", string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) reapIdleDocuments(ctx context.Context) {
	ttl := m.cfg.DocumentTTL.Duration
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for _, doc := range m.tracker.list() {
				if now.Sub(doc.LastAccessed) > ttl {
					_ = m.Close(context.Background(), doc.URI)
				}
			}
		}
	}
}
