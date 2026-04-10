package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/harsha/lspd/internal/config"
	internalformat "github.com/harsha/lspd/internal/format"
	"go.lsp.dev/protocol"
	"gopkg.in/yaml.v3"
)

type goldenConfig struct {
	Policy config.PolicyConfig `yaml:"policy"`
}

type goldenInput struct {
	Path        string                `json:"path"`
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
	Actions     map[string][]string   `json:"actions"`
}

type goldenSession struct {
	SessionID string `json:"session_id"`
}

type goldenExpected struct {
	Diagnostics []protocol.Diagnostic `json:"diagnostics"`
	CodeActions map[string][]string   `json:"code_actions,omitempty"`
}

func TestGoldenFixtures(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "test", "golden", "policy")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, entry.Name())

			var cfg goldenConfig
			mustReadYAML(t, filepath.Join(dir, "config.yaml"), &cfg)

			var input goldenInput
			mustReadJSON(t, filepath.Join(dir, "input.json"), &input)

			var session goldenSession
			mustReadJSON(t, filepath.Join(dir, "session.json"), &session)

			var expected goldenExpected
			mustReadJSON(t, filepath.Join(dir, "expected.json"), &expected)

			engine := New(cfg.Policy, func(_ context.Context, _ string, diagnostic protocol.Diagnostic) ([]string, error) {
				if input.Actions == nil {
					return nil, nil
				}
				return input.Actions[diagnostic.Message], nil
			})

			got := normalizeResult(engine.Apply(context.Background(), session.SessionID, input.Path, input.Diagnostics))
			expected = normalizeExpected(input, expected)

			gotBytes, _ := json.Marshal(got)
			wantBytes, _ := json.Marshal(expected)
			if string(gotBytes) != string(wantBytes) {
				t.Fatalf("mismatch\n got: %s\nwant: %s", gotBytes, wantBytes)
			}
		})
	}
}

func normalizeResult(result Result) Result {
	if result.CodeActions == nil {
		result.CodeActions = map[string][]string{}
	}
	return result
}

func normalizeExpected(input goldenInput, expected goldenExpected) goldenExpected {
	if expected.CodeActions == nil {
		expected.CodeActions = map[string][]string{}
		return expected
	}
	normalized := map[string][]string{}
	for key, actions := range expected.CodeActions {
		fingerprint := key
		for _, diagnostic := range input.Diagnostics {
			if diagnostic.Message == key {
				fingerprint = internalformat.Fingerprint(input.Path, diagnostic)
				break
			}
		}
		normalized[fingerprint] = actions
	}
	expected.CodeActions = normalized
	return expected
}

func mustReadJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("Unmarshal %s: %v", path, err)
	}
}

func mustReadYAML(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatalf("Unmarshal %s: %v", path, err)
	}
}
