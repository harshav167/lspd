package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/harshav167/lspd/internal/config"
	daemonlog "github.com/harshav167/lspd/internal/log"
	"github.com/harshav167/lspd/internal/lsp/router"
	"github.com/harshav167/lspd/internal/lsp/store"
	"go.lsp.dev/protocol"
)

type fixResponse struct {
	Path    string                `json:"path"`
	Line    int                   `json:"line"`
	Actions []protocol.CodeAction `json:"actions"`
}

func runFix(args []string) error {
	flags := flag.NewFlagSet("fix", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "output JSON")
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("fix requires PATH:LINE")
	}

	path, line, err := parseFixTarget(flags.Arg(0))
	if err != nil {
		return err
	}

	cfg, _, err := config.Load(*configPath, mustGetwd())
	if err != nil {
		return err
	}
	diagnosticStore := store.New()
	resolver := router.New(cfg, diagnosticStore, daemonlog.New(cfg), nil)
	defer resolver.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, _, err := resolver.Resolve(ctx, path)
	if err != nil {
		return err
	}
	doc, err := manager.EnsureOpen(ctx, path)
	if err != nil {
		return err
	}
	entry, _, _ := diagnosticStore.Wait(ctx, doc.URI, doc.Version, 2*time.Second)
	actions, err := manager.CodeAction(ctx, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: doc.URI},
		Range: protocol.Range{
			Start: protocol.Position{Line: uint32(max(line-1, 0)), Character: 0},
			End:   protocol.Position{Line: uint32(max(line-1, 0)), Character: 4096},
		},
		Context: protocol.CodeActionContext{Diagnostics: entry.Diagnostics},
	})
	if err != nil {
		return err
	}

	response := fixResponse{Path: path, Line: line, Actions: actions}
	if *jsonOut {
		data, marshalErr := json.MarshalIndent(response, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("%s:%d\n", path, line)
	if len(actions) == 0 {
		fmt.Println("no code actions")
		return nil
	}
	for _, action := range actions {
		fmt.Printf("- %s\n", action.Title)
	}
	return nil
}

func parseFixTarget(target string) (string, int, error) {
	index := strings.LastIndex(target, ":")
	if index <= 0 || index == len(target)-1 {
		return "", 0, fmt.Errorf("fix requires PATH:LINE")
	}
	line, err := strconv.Atoi(target[index+1:])
	if err != nil || line <= 0 {
		return "", 0, fmt.Errorf("invalid line in %q", target)
	}
	path, err := filepath.Abs(target[:index])
	if err != nil {
		return "", 0, err
	}
	return path, line, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
