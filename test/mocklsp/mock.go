package mocklsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Serve echoes empty initialize responses and empty diagnostics for tests.
func Serve(ctx context.Context, reader io.Reader, writer io.Writer) error {
	buffered := bufio.NewReader(reader)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := buffered.ReadString('\n')
		if err != nil {
			return err
		}
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "content-length:") {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(lower), ":", 2)
		if len(parts) != 2 {
			continue
		}
		length, parseErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if parseErr != nil || length <= 0 {
			continue
		}
		_, _ = buffered.ReadString('\n')
		body := make([]byte, length)
		if _, err := io.ReadFull(buffered, body); err != nil {
			return err
		}
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		response := map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"capabilities": map[string]any{}}}
		data, _ := json.Marshal(response)
		if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(data), data); err != nil {
			return err
		}
	}
}
