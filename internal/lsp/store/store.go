package store

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"go.lsp.dev/protocol"
)

// Entry is the latest known diagnostics for a document.
type Entry struct {
	URI              protocol.DocumentURI  `json:"uri"`
	Version          int32                 `json:"version"`
	PublishedVersion int32                 `json:"published_version"`
	Diagnostics      []protocol.Diagnostic `json:"diagnostics"`
	UpdatedAt        time.Time             `json:"updated_at"`
	Language         string                `json:"language"`
}

// WithDiagnostics returns a copy of the entry with cloned diagnostics.
func (e Entry) WithDiagnostics(diagnostics []protocol.Diagnostic) Entry {
	e.Diagnostics = cloneDiagnostics(diagnostics)
	return e
}

// Store tracks diagnostics and supports waiting for new versions.
type Store struct {
	mu      sync.RWMutex
	entries map[protocol.DocumentURI]Entry
	waiters map[protocol.DocumentURI][]waiter
}

type waiter struct {
	minVersion int32
	ch         chan struct{}
}

// New creates a diagnostic store.
func New() *Store {
	return &Store{entries: map[protocol.DocumentURI]Entry{}, waiters: map[protocol.DocumentURI][]waiter{}}
}

// Publish stores diagnostics and wakes waiters.
func (s *Store) Publish(uri protocol.DocumentURI, version int32, diagnostics []protocol.Diagnostic, language string) {
	s.mu.Lock()
	effectiveVersion := version
	if existing, ok := s.entries[uri]; ok && existing.Version > 0 {
		if effectiveVersion > 0 && effectiveVersion <= existing.Version {
			effectiveVersion = existing.Version + 1
		} else if effectiveVersion <= 0 {
			effectiveVersion = existing.Version + 1
		}
	} else if effectiveVersion <= 0 {
		effectiveVersion = 1
	}
	entry := Entry{URI: uri, Version: effectiveVersion, PublishedVersion: version, Diagnostics: cloneDiagnostics(diagnostics), UpdatedAt: time.Now(), Language: language}
	s.entries[uri] = entry
	waiters := s.waiters[uri]
	ready := make([]chan struct{}, 0, len(waiters))
	remaining := make([]waiter, 0, len(waiters))
	for _, waiter := range waiters {
		if effectiveVersion >= waiter.minVersion {
			ready = append(ready, waiter.ch)
			continue
		}
		remaining = append(remaining, waiter)
	}
	if len(remaining) == 0 {
		delete(s.waiters, uri)
	} else {
		s.waiters[uri] = remaining
	}
	s.mu.Unlock()
	for _, ch := range ready {
		close(ch)
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
	return copyEntry(entry), true
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
		out = append(out, copyEntry(entry))
	}
	return out
}

// Wait waits for the uri to reach at least minVersion or times out and returns the latest entry.
func (s *Store) Wait(ctx context.Context, uri protocol.DocumentURI, minVersion int32, timeout time.Duration) (Entry, bool, error) {
	if entry, ok := s.Peek(uri); ok && entry.Version >= minVersion {
		return entry, true, nil
	}
	w := waiter{minVersion: minVersion, ch: make(chan struct{})}
	s.mu.Lock()
	if entry, ok := s.entries[uri]; ok && entry.Version >= minVersion {
		s.mu.Unlock()
		return copyEntry(entry), true, nil
	}
	s.waiters[uri] = append(s.waiters[uri], w)
	s.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.ch:
		entry, ok := s.Peek(uri)
		return entry, ok, nil
	case <-timer.C:
		s.removeWaiter(uri, w.ch)
		entry, ok := s.Peek(uri)
		return entry, ok, context.DeadlineExceeded
	case <-ctx.Done():
		s.removeWaiter(uri, w.ch)
		entry, ok := s.Peek(uri)
		return entry, ok, ctx.Err()
	}
}

func (s *Store) removeWaiter(uri protocol.DocumentURI, ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	waiters := s.waiters[uri]
	if len(waiters) == 0 {
		return
	}
	filtered := waiters[:0]
	for _, waiter := range waiters {
		if waiter.ch != ch {
			filtered = append(filtered, waiter)
		}
	}
	if len(filtered) == 0 {
		delete(s.waiters, uri)
		return
	}
	s.waiters[uri] = filtered
}

func copyEntry(entry Entry) Entry {
	entry.Diagnostics = cloneDiagnostics(entry.Diagnostics)
	return entry
}

func cloneDiagnostics(in []protocol.Diagnostic) []protocol.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Diagnostic, len(in))
	copy(out, in)
	return out
}

// URIFromPath normalizes a filesystem path into the file URI shape used by the store.
func URIFromPath(path string) protocol.DocumentURI {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if len(cleaned) > 0 && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return protocol.DocumentURI("file://" + cleaned)
}
