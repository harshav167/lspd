package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/harshav167/lspd/internal/socket"
	"go.lsp.dev/protocol"
)

func runDiag(args []string) error {
	flags := flag.NewFlagSet("diag", flag.ContinueOnError)
	configPath := addConfigFlag(flags)
	jsonOut := flags.Bool("json", false, "output JSON")
	drain := flags.Bool("drain", false, "refresh diagnostics before returning")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("diag requires a file path")
	}
	op := "peek"
	if *drain {
		op = "drain"
	}
	response, err := requestConfiguredSocket(*configPath, socket.Request{Op: op, Path: flags.Arg(0)})
	if err != nil {
		return err
	}
	if *jsonOut {
		data, marshalErr := json.MarshalIndent(response, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(data))
		return nil
	}
	if response.Entry == nil {
		fmt.Println("no diagnostics")
		return nil
	}
	fmt.Printf("%s\n", response.Entry.URI)
	fmt.Printf("language=%s version=%d updated=%s diagnostics=%d\n", response.Entry.Language, response.Entry.Version, response.Entry.UpdatedAt.Format(time.RFC3339), len(response.Entry.Diagnostics))
	for _, diagnostic := range response.Entry.Diagnostics {
		fmt.Printf(
			"- %s %d:%d %s",
			severityLabel(diagnostic.Severity),
			diagnostic.Range.Start.Line+1,
			diagnostic.Range.Start.Character+1,
			diagnostic.Message,
		)
		if diagnostic.Source != "" {
			fmt.Printf(" (%s)", diagnostic.Source)
		}
		fmt.Println()
		if actions := response.CodeActions[diagnostic.Message]; len(actions) > 0 {
			for _, action := range actions {
				fmt.Printf("    fix: %s\n", action)
			}
		}
	}
	return nil
}

func severityLabel(severity protocol.DiagnosticSeverity) string {
	switch severity {
	case protocol.DiagnosticSeverityError:
		return "error"
	case protocol.DiagnosticSeverityWarning:
		return "warning"
	case protocol.DiagnosticSeverityInformation:
		return "info"
	case protocol.DiagnosticSeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}
