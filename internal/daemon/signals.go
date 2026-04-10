package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// WatchSignals binds SIGHUP/SIGINT/SIGTERM to the provided callbacks.
func WatchSignals(ctx context.Context, onReload func(), onStop func()) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer signal.Stop(signals)
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-signals:
				switch sig {
				case syscall.SIGHUP:
					onReload()
				default:
					onStop()
					return
				}
			}
		}
	}()
}
