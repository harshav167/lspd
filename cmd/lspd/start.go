package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/harshav167/lspd/internal/daemon"
	"github.com/harshav167/lspd/internal/socket"
)

func runStart(args []string) error {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	foreground := flags.Bool("foreground", false, "run in the foreground")
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	if !*foreground && os.Getenv(daemonizedEnvVar) == "" {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		cmdArgs := append([]string{"start", "--foreground"}, args...)
		cmd := exec.Command(executable, cmdArgs...)
		cmd.Env = append(os.Environ(), daemonizedEnvVar+"=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Start()
	}

	app, err := daemon.New(*configPath, mustGetwd())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.SetCancel(cancel)
	stop := make(chan struct{})
	daemon.WatchSignals(ctx, func() {
		_ = app.Reload(context.Background())
	}, func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
		cancel()
	})
	if err := app.Start(ctx); err != nil {
		if port, ok := existingPort(*configPath); ok {
			fmt.Println(port)
			return nil
		}
		return err
	}
	fmt.Println(app.Port())
	select {
	case <-time.After(24 * time.Hour):
		return app.Close(context.Background())
	case <-stop:
		return app.Close(context.Background())
	case <-ctx.Done():
		return app.Close(context.Background())
	}
}

func existingPort(configPath string) (int, bool) {
	response, err := requestConfiguredSocket(configPath, socket.Request{Op: "status"})
	if err != nil {
		return 0, false
	}
	if port, ok := intValue(response.Status["port"]); ok && port > 0 {
		return port, true
	}
	return 0, false
}
