package watcher

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Handler is called for file changes after debouncing.
type Handler func(context.Context, string) error

// Watcher debounces file system events and forwards them to the handler.
type Watcher struct {
	fs       *fsnotify.Watcher
	debounce time.Duration
	handler  Handler
	mu       sync.Mutex
	pending  map[string]*time.Timer
	watched  map[string]struct{}
	onError  func(error)
}

// New creates a watcher.
func New(debounce time.Duration, handler Handler) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fs:       fs,
		debounce: debounce,
		handler:  handler,
		pending:  map[string]*time.Timer{},
		watched:  map[string]struct{}{},
	}, nil
}

// Add adds a path to the watcher.
func (w *Watcher) Add(path string) error {
	cleaned := filepath.Clean(path)
	info, err := os.Stat(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return w.addDir(filepath.Dir(cleaned))
	}
	return filepath.WalkDir(cleaned, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if shouldIgnorePath(current) {
			if current == cleaned {
				return nil
			}
			return filepath.SkipDir
		}
		return w.addDir(current)
	})
}

// Run runs the watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				w.stopAllTimers()
				_ = w.fs.Close()
				return
			case event, ok := <-w.fs.Events:
				if !ok {
					return
				}
				w.handleEvent(ctx, event)
			case err, ok := <-w.fs.Errors:
				if !ok {
					return
				}
				if err != nil && w.onError != nil {
					w.onError(err)
				} else if err != nil {
					slog.Error("watcher error", "error", err)
				}
			}
		}
	}()
}

func (w *Watcher) addDir(path string) error {
	cleaned := filepath.Clean(path)
	if shouldIgnorePath(cleaned) {
		return nil
	}
	w.mu.Lock()
	if _, ok := w.watched[cleaned]; ok {
		w.mu.Unlock()
		return nil
	}
	w.watched[cleaned] = struct{}{}
	if err := w.fs.Add(cleaned); err != nil {
		if os.IsNotExist(err) {
			delete(w.watched, cleaned)
			w.mu.Unlock()
			return nil
		}
		delete(w.watched, cleaned)
		w.mu.Unlock()
		return err
	}
	w.mu.Unlock()
	return nil
}

func (w *Watcher) handleEvent(ctx context.Context, event fsnotify.Event) {
	path := filepath.Clean(event.Name)
	if shouldIgnorePath(path) {
		return
	}
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			_ = w.Add(path)
			return
		}
	}
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		w.mu.Lock()
		delete(w.watched, path)
		w.mu.Unlock()
	}
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		return
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return
	}
	if err != nil {
		return
	}
	w.schedule(ctx, path)
}

func (w *Watcher) schedule(ctx context.Context, path string) {
	w.mu.Lock()
	if timer, ok := w.pending[path]; ok {
		timer.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(w.debounce, func() {
		defer func() {
			w.mu.Lock()
			if current, ok := w.pending[path]; ok && current == timer {
				delete(w.pending, path)
			}
			w.mu.Unlock()
		}()
		_ = w.handler(ctx, path)
	})
	w.pending[path] = timer
	w.mu.Unlock()
}

func (w *Watcher) stopAllTimers() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for path, timer := range w.pending {
		timer.Stop()
		delete(w.pending, path)
	}
}

func shouldIgnorePath(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		switch part {
		case ".git", "node_modules", ".venv", "dist", "build", "target":
			return true
		}
	}
	return false
}
