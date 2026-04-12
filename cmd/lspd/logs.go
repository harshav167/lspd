package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type logLine struct {
	Time string `json:"time"`
}

func runLogs(args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := flags.Bool("follow", false, "follow the log file")
	since := flags.Duration("since", 0, "show only log lines newer than the given duration")
	configPath := addConfigFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := loadCLIConfig(*configPath)
	if err != nil {
		return err
	}
	path := cfg.LogFile
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) || !*follow {
			return err
		}
		data = nil
	}
	cutoff := time.Time{}
	if *since > 0 {
		cutoff = time.Now().Add(-*since)
	}
	printFilteredLogChunk(data, cutoff)
	if !*follow {
		return nil
	}

	file, err := os.Open(path)
	if err != nil && os.IsNotExist(err) {
		for {
			time.Sleep(500 * time.Millisecond)
			file, err = os.Open(path)
			if err == nil {
				break
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err != nil {
		return err
	}
	defer file.Close()

	offset := int64(len(data))
	for {
		if info, statErr := file.Stat(); statErr == nil && info.Size() < offset {
			offset = 0
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		chunk, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		if len(chunk) > 0 {
			printFilteredLogChunk(chunk, cutoff)
			offset += int64(len(chunk))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func printFilteredLogChunk(chunk []byte, cutoff time.Time) {
	scanner := bufio.NewScanner(bytes.NewReader(chunk))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if shouldPrintLogLine(line, cutoff) {
			fmt.Println(line)
		}
	}
}

func shouldPrintLogLine(line string, cutoff time.Time) bool {
	if cutoff.IsZero() {
		return true
	}
	var parsed logLine
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return true
	}
	if parsed.Time == "" {
		return true
	}
	timestamp, err := time.Parse(time.RFC3339Nano, parsed.Time)
	if err != nil {
		return true
	}
	return !timestamp.Before(cutoff)
}
