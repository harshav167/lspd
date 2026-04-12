package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestShouldIgnorePathMatchesKnownDirectories(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/tmp/project/.git/config",
		"/tmp/project/node_modules/pkg/index.js",
		"/tmp/project/.venv/bin/python",
	} {
		if !shouldIgnorePath(path) {
			t.Fatalf("expected path %q to be ignored", path)
		}
	}
	if shouldIgnorePath("/tmp/project/src/main.go") {
		t.Fatal("did not expect regular source path to be ignored")
	}
}

func TestAddDirTracksDirectoriesOnce(t *testing.T) {
	t.Parallel()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer fsWatcher.Close()

	w := &Watcher{
		fs:      fsWatcher,
		handler: func(context.Context, string) error { return nil },
		pending: map[string]*time.Timer{},
		watched: map[string]struct{}{},
	}

	root := t.TempDir()
	if err := w.addDir(root); err != nil {
		t.Fatalf("addDir: %v", err)
	}
	if err := w.addDir(root); err != nil {
		t.Fatalf("addDir second call: %v", err)
	}
	if len(w.watched) != 1 {
		t.Fatalf("expected one watched directory, got %d", len(w.watched))
	}
}

func TestAddWalksNestedDirectories(t *testing.T) {
	t.Parallel()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer fsWatcher.Close()

	w := &Watcher{
		fs:      fsWatcher,
		handler: func(context.Context, string) error { return nil },
		pending: map[string]*time.Timer{},
		watched: map[string]struct{}{},
	}

	root := t.TempDir()
	nested := filepath.Join(root, "pkg", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := w.Add(root); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(w.watched) < 3 {
		t.Fatalf("expected nested directories to be watched, got %d", len(w.watched))
	}
}
