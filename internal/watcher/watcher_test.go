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

func TestSyncRootsAddsRootsFromSnapshotter(t *testing.T) {
	t.Parallel()

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer fsWatcher.Close()

	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &Watcher{
		fs:       fsWatcher,
		debounce: 10 * time.Millisecond,
		handler:  func(context.Context, string) error { return nil },
		pending:  map[string]*time.Timer{},
		watched:  map[string]struct{}{},
	}

	done := make(chan struct{})
	go func() {
		w.SyncRoots(ctx, func() []string { return []string{root, root} })
		close(done)
	}()

	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		w.mu.Lock()
		count := len(w.watched)
		w.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected one synced root, got %d", count)
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()
	<-done
}
