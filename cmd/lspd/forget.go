package main

import (
	"flag"
	"fmt"

	"github.com/harshav167/lspd/internal/socket"
)

func runForget(args []string) error {
	flags := flag.NewFlagSet("forget", flag.ContinueOnError)
	configPath := addConfigFlag(flags)
	sessionID := flags.String("session", "", "session id to forget")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *sessionID == "" {
		return fmt.Errorf("forget requires --session")
	}
	_, err := requestConfiguredSocket(*configPath, socket.Request{
		Op:        "forget",
		SessionID: *sessionID,
	})
	return err
}
