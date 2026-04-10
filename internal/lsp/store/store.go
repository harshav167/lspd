package store

import (
	"context"
	"sync"
	"time"

	"go.lsp.dev/protocol"
)

// Entry is the latest known diagnostics for a document.
type Entry struct {
	URI         protocol.DocumentURI  `json:"uri"`
	Version     int32                 `json:"version"`
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Language    string                `json:"language"`
}

// Store tracks diagnostics and supports waiting for new versions.
type Store struct {
	mu      sync.RWMutex
	entries map[protocol.DocumentURI]Entry
	waiters map[protocol.DocumentURI][]chan struct{}
}

// New creates a diagnostic store.
func New() *Store {
	return &Store{entries: map[protocol.DocumentURI]Entry{}, waiters: map[protocol.DocumentURI][]chan struct{}{}}
}

// Publish stores diagnostics and wakes waiters.
func (s *Store) Publish(uri protocol.DocumentURI, version int32, diagnostics []protocol.Diagnostic, language string) {
	s.mu.Lock()
	effectiveVersion := version
	if effectiveVersion == 0 {
		if existing, ok := s.entries[uri]; ok && existing.Version > 0 {
			effectiveVersion = existing.Version + 1
		} else {
			effectiveVersion = 1
		}
	}
	s.entries[uri] = Entry{URI: uri, Version: effectiveVersion, Diagnostics: cloneDiagnostics(diagnostics), UpdatedAt: time.Now(), Language: language}
	waiters := s.waiters[uri]
	delete(s.waiters, uri)
	s.mu.Unlock()
	for _, waiter := range waiters {
		close(waiter)
	}
}

// Peek returns the latest entry.
func (s *Store) Peek(uri protocol.DocumentURI) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[uri]
	if !ok {
		return Entry{}, false
	}
	entry.Diagnostics = cloneDiagnostics(entry.Diagnostics)
	return entry, true
}

// Forget forgets a URI.
func (s *Store) Forget(uri protocol.DocumentURI) {
	s.mu.Lock()
	delete(s.entries, uri)
	delete(s.waiters, uri)
	s.mu.Unlock()
}

// Snapshot returns all entries.
func (s *Store) Snapshot() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		entry.Diagnostics = cloneDiagnostics(entry.Diagnostics)
		out = append(out, entry)
	}
	return out
}

// Wait waits for the uri to reach at least minVersion or times out and returns the latest entry.
func (s *Store) Wait(ctx context.Context, uri protocol.DocumentURI, minVersion int32, timeout time.Duration) (Entry, bool, error) {
	if entry, ok := s.Peek(uri); ok && entry.Version >= minVersion {
		return entry, true, nil
	}
	waiter := make(chan struct{})
	s.mu.Lock()
	if entry, ok := s.entries[uri]; ok && entry.Version >= minVersion {
		s.mu.Unlock()
		return entry, true, nil
	}
	s.waiters[uri] = append(s.waiters[uri], waiter)
	s.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waiter:
		entry, ok := s.Peek(uri)
		return entry, ok, nil
	case <-timer.C:
		entry, ok := s.Peek(uri)
		return entry, ok, context.DeadlineExceeded
	case <-ctx.Done():
		entry, ok := s.Peek(uri)
		return entry, ok, ctx.Err()
	}
}

func cloneDiagnostics(in []protocol.Diagnostic) []protocol.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Diagnostic, len(in))
	copy(out, in)
	return out
}
