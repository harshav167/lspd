package socket

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/harsha/lspd/internal/lsp/store"
)

// Server serves unix socket requests from the read hook and CLI.
type Server struct {
	path     string
	handler  *handler
	listener net.Listener
	wg       sync.WaitGroup
}

// Callbacks customize socket semantics.
type Callbacks struct {
	Peek   func(context.Context, Request) (store.Entry, bool, error)
	Drain  func(context.Context, Request) (store.Entry, bool, error)
	Forget func(Request)
	Status func() map[string]any
	Reload func(context.Context) error
	Touch  func()
}

// NewServer creates a socket server.
func NewServer(path string, diagnosticStore *store.Store, callbacks Callbacks) *Server {
	return &Server{path: path, handler: &handler{
		store:  diagnosticStore,
		peek:   callbacks.Peek,
		drain:  callbacks.Drain,
		forget: callbacks.Forget,
		reload: callbacks.Reload,
		status: callbacks.Status,
		touch:  callbacks.Touch,
	}}
}

// Start starts the socket server.
func (s *Server) Start(ctx context.Context) error {
	_ = os.Remove(s.path)
	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.path, err)
	}
	s.listener = listener
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer conn.Close()
				var request Request
				if err := json.NewDecoder(conn).Decode(&request); err != nil {
					_ = json.NewEncoder(conn).Encode(Response{Message: err.Error()})
					return
				}
				response := s.handler.handle(ctx, request)
				_ = json.NewEncoder(conn).Encode(response)
			}()
		}
	}()
	return nil
}

// Close closes the socket listener.
func (s *Server) Close() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	return os.Remove(s.path)
}
