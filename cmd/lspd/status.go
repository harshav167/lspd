package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/harshav167/lspd/internal/socket"
)

func runStatus(args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "output json")
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	response, err := requestConfiguredSocket(*configPath, socket.Request{Op: "status"})
	if err != nil {
		return err
	}
	if *jsonOut {
		data, _ := json.MarshalIndent(response.Status, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	status := response.Status
	if status == nil {
		return fmt.Errorf("status payload missing")
	}
	fmt.Printf(
		"lspd %s   pid %s   uptime %s   idle %s\n",
		stringValue(status["version"]),
		stringValue(status["pid"]),
		stringValue(status["uptime"]),
		stringValue(status["idle"]),
	)
	fmt.Printf("%s   socket %s\n", stringValue(status["mcp_url"]), stringValue(status["socket_path"]))
	if configPath := stringValue(status["config_path"]); configPath != "" {
		fmt.Printf("Config: %s", configPath)
		if generation, ok := intValue(status["config_generation"]); ok {
			fmt.Printf("   generation %d", generation)
		}
		fmt.Println()
	}
	if metrics := mapValue(status["metrics"]); metrics != nil && metrics["enabled"] == true {
		fmt.Printf("Metrics: %s\n", stringValue(metrics["url"]))
		if debugURL := stringValue(metrics["debug_url"]); debugURL != "" {
			fmt.Printf("Debug:   %s\n", debugURL)
		}
	}
	fmt.Println()
	fmt.Println("Languages:")
	for _, item := range sliceValue(status["language_states"]) {
		state := mapValue(item)
		if state == nil {
			continue
		}
		lastPublish := relativeTime(state["last_publish"])
		if lastPublish == "" {
			lastPublish = "never"
		}
		fmt.Printf(
			"  %-12s pid %-6s docs %-3s %-10s last publish %s root %s\n",
			stringValue(state["language"]),
			stringValue(state["pid"]),
			stringValue(state["document_count"]),
			stringValue(state["supervisor_state"]),
			lastPublish,
			stringValue(state["root"]),
		)
	}
	if policy := mapValue(status["policy"]); policy != nil {
		fmt.Println()
		fmt.Printf(
			"Policy: min=%s per_file=%s per_turn=%s code_actions=%s\n",
			stringValue(policy["minimum_severity"]),
			stringValue(policy["max_per_file"]),
			stringValue(policy["max_per_turn"]),
			stringValue(policy["attach_code_actions"]),
		)
	}
	if store := mapValue(status["diagnostic_store"]); store != nil {
		fmt.Printf(
			"Diagnostic store: entries=%s total_diagnostics=%s\n",
			stringValue(store["entries"]),
			stringValue(store["total_diagnostics"]),
		)
	}
	return nil
}
