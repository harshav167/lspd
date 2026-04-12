package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/harshav167/lspd/internal/config"
	"github.com/harshav167/lspd/internal/socket"
)

func runStop(args []string) error {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	configPath := addConfigFlag(flags)
	force := flags.Bool("force", false, "force kill if graceful shutdown times out")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	pidBytes, err := os.ReadFile(filepath.Join(cfg.RunDir, "lspd.pid"))
	if err == nil {
		if pid, scanErr := strconv.Atoi(strings.TrimSpace(string(pidBytes))); scanErr == nil {
			if killErr := syscall.Kill(pid, syscall.SIGTERM); killErr != nil {
				if killErr == syscall.ESRCH {
					cleanupRuntimeFiles(cfg)
					return nil
				}
				return killErr
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, pingErr := requestSocket(cfg.Socket.Path, socket.Request{Op: "ping"}); pingErr != nil {
					cleanupRuntimeFiles(cfg)
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			if *force {
				if killErr := syscall.Kill(pid, syscall.SIGKILL); killErr != nil && killErr != syscall.ESRCH {
					return killErr
				}
				time.Sleep(100 * time.Millisecond)
				cleanupRuntimeFiles(cfg)
				return nil
			}
			return fmt.Errorf("daemon did not exit within 5s; rerun with --force")
		}
	}
	if _, err := requestSocket(cfg.Socket.Path, socket.Request{Op: "ping"}); err != nil {
		cleanupRuntimeFiles(cfg)
		return nil
	}
	return fmt.Errorf("daemon is responding but pid file is unavailable")
}

func cleanupRuntimeFiles(cfg config.Config) {
	_ = os.Remove(cfg.Socket.Path)
	_ = os.Remove(filepath.Join(cfg.RunDir, "lspd.port"))
	_ = os.Remove(filepath.Join(cfg.RunDir, "lspd.pid"))
}
