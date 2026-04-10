# Configuration

`lspd` loads configuration from these locations, in order:

1. an explicit `--config` path
2. `~/.config/lspd/config.yaml`
3. the nearest `lspd.yaml` walking upward from the current working directory

Most keys are optional because `internal/config.Default()` supplies defaults. The `languages` map is logically required if you want language servers enabled, and each language entry must declare at least `command` and `extensions`.

## Key settings

- `mcp.host` — string, optional, default `127.0.0.1`; host interface for the MCP StreamableHTTP server.
- `mcp.endpoint` — string, optional, default `/mcp`; HTTP path exposed to Droid.
- `mcp.session_header` — string, optional, default `X-Droid-Session-Id`; header used to track session-scoped diagnostic dedup.
- `socket.path` — string, required in practice, default `~/.factory/run/lspd.sock`; unix socket path used by CLI commands and `lsp-read-hook`.
- `policy.max_per_file` — integer, optional, default `20`; max diagnostics surfaced for a single file in one response. Must be `> 0`.
- `policy.max_per_turn` — integer, optional, default `50`; max diagnostics surfaced across one response. Must be `> 0`.
- `policy.minimum_severity` — integer, optional, default `1`; lowest LSP severity kept by policy (`1=error`, `2=warning`, `3=information`, `4=hint` in LSP terms before Droid remapping).
- `policy.allowed_sources` — list of strings, optional; if set, only diagnostics from these sources are allowed.
- `policy.denied_sources` — list of strings, optional; diagnostics from these sources are dropped.
- `policy.attach_code_actions` — boolean, optional, default `true`; whether quick-fix previews are attached when available.
- `languages` — map, required for real usage; per-language server definitions keyed by logical language name.

## Languages section shape

Each language entry may contain:

- `command` — string, required; executable name, e.g. `gopls`
- `args` — string array, optional; command-line arguments, e.g. `["--stdio"]`
- `extensions` — string array, required; file extensions routed to this language
- `root_markers` — string array, optional; project-root detection markers
- `settings` — object, optional; sent via `workspace/didChangeConfiguration`
- `initialization_options` — object, optional; sent in the LSP initialize request
- `workspace_folders` — boolean, optional; whether workspace folders should be sent
- `warmup` — boolean, optional; whether the manager should eagerly warm the server
- `max_restarts` — integer, optional; restart cap for the supervisor
- `restart_window` — duration string, optional; e.g. `10m`
- `document_ttl` — duration string, optional; e.g. `15m`

## Example

```yaml
mcp:
  host: 127.0.0.1
  endpoint: /mcp
  session_header: X-Droid-Session-Id

socket:
  path: /Users/you/.factory/run/lspd.sock

policy:
  max_per_file: 20
  max_per_turn: 50
  minimum_severity: 1
  attach_code_actions: true

languages:
  ts:
    command: typescript-language-server
    args: ["--stdio"]
    extensions: [".ts", ".tsx", ".js", ".jsx"]
    root_markers: ["tsconfig.json", "package.json", ".git"]

  go:
    command: gopls
    extensions: [".go", ".mod", ".sum"]
    root_markers: ["go.mod", ".git"]
    settings:
      gopls:
        staticcheck: true
```
