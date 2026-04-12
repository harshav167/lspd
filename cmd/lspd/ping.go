package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/harshav167/lspd/internal/socket"
)

func runPing(args []string) error {
	flags := flag.NewFlagSet("ping", flag.ContinueOnError)
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	response, err := requestConfiguredSocket(*configPath, socket.Request{Op: "ping"})
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", response.Message, response.Time.Format(time.RFC3339))
	return nil
}
