package client

import (
	"sync"
	"time"

	"go.lsp.dev/protocol"
)

type trackedDocument struct {
	Path         string
	URI          protocol.DocumentURI
	LanguageID   protocol.LanguageIdentifier
	Version      int32
	Content      string
	LastAccessed time.Time
}

// Document is the manager-owned snapshot of an open text document.
type Document = trackedDocument

type documentTracker struct {
	mu   sync.RWMutex
	docs map[protocol.DocumentURI]trackedDocument
}

func newDocumentTracker() *documentTracker {
	return &documentTracker{docs: map[protocol.DocumentURI]trackedDocument{}}
}

func (t *documentTracker) get(uri protocol.DocumentURI) (trackedDocument, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	doc, ok := t.docs[uri]
	return doc, ok
}

func (t *documentTracker) put(doc trackedDocument) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.docs[doc.URI] = doc
}

func (t *documentTracker) delete(uri protocol.DocumentURI) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.docs, uri)
}

func (t *documentTracker) list() []trackedDocument {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]trackedDocument, 0, len(t.docs))
	for _, doc := range t.docs {
		out = append(out, doc)
	}
	return out
}
