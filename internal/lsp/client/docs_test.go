package client

import (
	"testing"
	"time"

	"go.lsp.dev/protocol"
)

func TestDocumentTrackerCRUD(t *testing.T) {
	t.Parallel()

	tracker := newDocumentTracker()
	doc := trackedDocument{
		Path:         "/tmp/example.go",
		URI:          protocol.DocumentURI("file:///tmp/example.go"),
		LanguageID:   protocol.LanguageIdentifier("go"),
		Version:      1,
		Content:      "package main\n",
		LastAccessed: time.Now(),
	}

	tracker.put(doc)

	got, ok := tracker.get(doc.URI)
	if !ok {
		t.Fatal("expected document to exist")
	}
	if got.Path != doc.Path || got.Version != doc.Version {
		t.Fatalf("unexpected document: %#v", got)
	}

	list := tracker.list()
	if len(list) != 1 || list[0].URI != doc.URI {
		t.Fatalf("unexpected tracked docs: %#v", list)
	}

	tracker.delete(doc.URI)
	if _, ok := tracker.get(doc.URI); ok {
		t.Fatal("expected document to be deleted")
	}
}
