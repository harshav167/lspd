package main

import (
	"flag"
	"fmt"

	"github.com/harshav167/lspd/internal/socket"
)

func runReload(args []string) error {
	flags := flag.NewFlagSet("reload", flag.ContinueOnError)
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	response, err := requestConfiguredSocket(*configPath, socket.Request{Op: "reload"})
	if err != nil {
		return err
	}
	fmt.Println(response.Message)
	return nil
}
