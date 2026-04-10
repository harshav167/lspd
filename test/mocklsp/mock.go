package mocklsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		if !strings.HasPrefix(strings.ToLower(line), "content-length:") {
			continue
		}
		var length int
		fmt.Sscanf(line, "Content-Length: %d", &length)
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
