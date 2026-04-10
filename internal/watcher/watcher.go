package watcher

import (
	"context"
	"path/filepath"
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
}

// New creates a watcher.
func New(debounce time.Duration, handler Handler) (*Watcher, error) {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{fs: fs, debounce: debounce, handler: handler, pending: map[string]*time.Timer{}}, nil
}

// Add adds a path to the watcher.
func (w *Watcher) Add(path string) error {
	return w.fs.Add(path)
}

// Run runs the watcher loop.
func (w *Watcher) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = w.fs.Close()
				return
			case event := <-w.fs.Events:
				path := filepath.Clean(event.Name)
				w.mu.Lock()
				if timer, ok := w.pending[path]; ok {
					timer.Stop()
				}
				w.pending[path] = time.AfterFunc(w.debounce, func() {
					_ = w.handler(ctx, path)
				})
				w.mu.Unlock()
			case <-w.fs.Errors:
			}
		}
	}()
}
