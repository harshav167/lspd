package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/harsha/lspd/internal/config"
	"github.com/harsha/lspd/internal/lsp/router"
	"github.com/harsha/lspd/internal/lsp/store"
	compattools "github.com/harsha/lspd/internal/mcp/tools/compat"
	navtools "github.com/harsha/lspd/internal/mcp/tools/nav"
	"github.com/harsha/lspd/internal/metrics"
	"github.com/harsha/lspd/internal/policy"
	mcpsdk "github.com/mark3labs/mcp-go/server"
)

// Dependencies are shared across all MCP handlers.
type Dependencies struct {
	Config  *config.Manager
	Router  *router.Router
	Store   *store.Store
	Policy  *policy.Engine
	Logger  *slog.Logger
	Touch   func()
	Metrics *metrics.Registry
}

// Server exposes the daemon over MCP StreamableHTTP.
type Server struct {
	cfg      config.Config
	mcp      *mcpsdk.MCPServer
	http     *http.Server
	listener net.Listener
	cancel   context.CancelFunc
}

// NewServer creates the MCP server.
func NewServer(cfg config.Config, deps Dependencies) *Server {
	srv := mcpsdk.NewMCPServer("lspd", "0.1.0", mcpsdk.WithToolCapabilities(true))
	compattools.Register(srv, compattools.Dependencies{
		Router:        deps.Router,
		Store:         deps.Store,
		Policy:        deps.Policy,
		SessionIDFrom: SessionIDFromContext,
		Metrics:       deps.Metrics,
	})
	navtools.Register(srv, navtools.Dependencies{
		Router:  deps.Router,
		Store:   deps.Store,
		Policy:  deps.Policy,
		Metrics: deps.Metrics,
	})
	handler := mcpsdk.NewStreamableHTTPServer(
		srv,
		mcpsdk.WithHeartbeatInterval(5*time.Second),
		mcpsdk.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if deps.Touch != nil {
				deps.Touch()
			}
			return WithSessionID(ctx, RequestSessionID(r, cfg.MCP.SessionHeader))
		}),
	)
	return &Server{
		cfg: cfg,
		mcp: srv,
		http: &http.Server{
			Handler: handler,
		},
	}
}

// Start starts the MCP server on a local random port.
func (s *Server) Start() (int, error) {
	lc := net.ListenConfig{KeepAlive: 30 * time.Second}
	listener, err := lc.Listen(context.Background(), "tcp", net.JoinHostPort(s.cfg.MCP.Host, "0"))
	if err != nil {
		return 0, fmt.Errorf("listen mcp: %w", err)
	}
	s.listener = listener
	heartbeatCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		_ = s.http.Serve(listener)
	}()
	go s.heartbeatLoop(heartbeatCtx)
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// Close shuts the MCP server down.
func (s *Server) Close(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.http != nil {
		return s.http.Shutdown(ctx)
	}
	return nil
}

func (s *Server) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mcp.SendNotificationToAllClients("notifications/heartbeat", map[string]any{
				"timestamp": time.Now().UnixMilli(),
			})
		}
	}
}
