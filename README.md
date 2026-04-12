# lspd

**Give your AI coding agent the same LSP feedback a human gets from their IDE — without needing an IDE.**

`lspd` is a lightweight Go daemon that feeds real-time compiler diagnostics and semantic code-navigation tools to AI coding agents. After every file write and every file read, the agent sees what broke — the same way you see squiggly red lines the moment you stop typing.

Works with [Droid](https://factory.ai), [Codex](https://github.com/openai/codex), and any agent harness that supports [MCP](https://modelcontextprotocol.io) or post-tool-use hooks.

---

## Install

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/install.sh | sh
```

That's it. Now just run your agent normally:

```sh
droid
```

The installer:
1. Downloads `lspd` and `lsp-read-hook` binaries for your platform
2. Writes daemon config to `~/.factory/hooks/lsp/lspd.yaml`
3. Merges diagnostic hooks into your `~/.factory/settings.json` (non-destructive — your existing hooks are preserved)
4. Enables IDE auto-discovery so Droid finds lspd automatically

No wrapper script. No alias. No environment variables to set. lspd starts via a SessionStart hook, registers itself via a lock file at `~/.factory/ide/`, and Droid auto-discovers it — the same mechanism the VS Code extension uses.

### Update

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/install.sh | sh
```

Same command. It downloads the latest binaries and overwrites the old ones. Config is preserved.

### Uninstall

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/uninstall.sh | sh
```

Removes binaries, hooks, and runtime state. Your config at `~/.factory/hooks/lsp/lspd.yaml` is kept unless you pass `--purge`.

---

## Why not just use VS Code?

You can. If you're inside VS Code, Cursor, or Windsurf with the Factory extension, you already have diagnostics. The extension and lspd use the same integration seam — they coexist without conflict.

**lspd exists for everywhere else:**

| Scenario | VS Code extension | lspd |
|---|---|---|
| Terminal (iTerm2, kitty, Warp, tmux) | Not available | Works |
| SSH into a remote server | Need VS Code Remote | Works natively |
| CI/CD pipelines | No IDE | Works |
| Docker containers | Only if VS Code manages them | Works |
| Low-resource machines | VS Code = 500MB+ RAM | lspd = ~20MB binary |
| Headless agent runs | Extension can't attach | Works |

When both VS Code and lspd are present, Droid prefers the real IDE for writes and lspd's Read hook still adds value — because Droid's `read-cli` never calls `fetchDiagnostics` regardless of which IDE is connected. The Read hook fills that gap in every scenario.

---

## What it does

### 1. Diagnostics after every write

When the agent edits a file, Droid's built-in `fetchDiagnostics` pipeline calls lspd. lspd queries the language server (gopls, pyright, typescript-language-server, rust-analyzer, or clangd) and returns the diagnostics. Droid diffs before vs. after and shows only the new errors:

```
<system-reminder>
New errors detected after editing users.ts:
Errors:
  - Line 12: Cannot find name 'fetchUsers' (ts)
  - Line 18: Expected 2 arguments, but got 1 (ts)
Warnings:
  - Line 25: 'tmp' is declared but its value is never read (ts)
</system-reminder>
```

No `go build`. No `tsc --noEmit`. No compile-error-patch-compile loop.

### 2. Diagnostics after every read

When the agent reads a file, a PostToolUse hook queries lspd and injects known diagnostics as a system message. The agent sees errors *before it starts planning changes* — proactive awareness, not reactive discovery.

### 3. Semantic navigation tools

10 LSP-powered tools the model can call directly:

| Tool | Replaces |
|---|---|
| `lspDefinition` | Grepping for `function foo` |
| `lspReferences` | `grep -r "foo" src/` |
| `lspHover` | Reading source to infer the type |
| `lspWorkspaceSymbol` | `grep -r` across the repo |
| `lspDocumentSymbol` | Reading a file to understand its structure |
| `lspCodeActions` | Guessing what import to add |
| `lspRename` | Manual search-and-replace |
| `lspFormat` | Shelling out to `prettier`/`gofmt` |
| `lspCallHierarchy` | Grepping for function names |
| `lspTypeHierarchy` | Manually tracing inheritance |

---

## The problem this solves

Without inline diagnostics, AI agents spend ~40% of their build session in this loop:

```
write file → compile → error → patch → compile → different error → patch → compile → ...
```

From a real session building this project — **eleven compile cycles** for errors that were all visible to the language server within 200ms of each edit:

```
go build → undefined: protocol.TypeScript → patch → rebuild
         → undefined: protocol.Python → patch → rebuild
         → undefined: net → add import → rebuild
         → m.server.Shutdown returns 1 value → fix → rebuild
         → undefined: protocol.ErrUnknownProtocol → fix → rebuild
         → "strings" imported and not used → remove → rebuild
         → "go.lsp.dev/protocol" imported and not used → remove → rebuild
         → type mismatch → export + alias → rebuild
         → "go.lsp.dev/protocol" imported and not used → remove → rebuild
```

With lspd: zero compile cycles. Each error surfaces as a `<system-reminder>` attached to the edit that caused it.

---

## How it works

```
  Agent harness (Droid / Codex / etc.)        lspd daemon
  ┌──────────────────┐                       ┌────────────────────┐
  │ Write/Edit/Create │── getIdeDiagnostics ─►│ MCP server         │
  │ (native pipeline) │◄─ {diagnostics:[]} ──│ (StreamableHTTP)   │
  │                   │                       │                    │
  │ Read              │                       │ ┌────────────────┐ │
  │ (PostToolUse hook)│── socket peek ───────►│ │ Diagnostic     │ │
  │                   │◄─ <system-reminder> ──│ │ store          │ │
  │                   │                       │ └───────┬────────┘ │
  │ lspDefinition     │── MCP tool call ─────►│         │          │
  │ lspReferences     │◄─ {locations:[]} ────│    publishDiag.    │
  │ lspHover, etc.    │                       │         │          │
  └──────────────────┘                       │ ┌───────┴────────┐ │
                                              │ │ LSP pool       │ │
                                              │ │ ts py go rs cc │ │
                                              │ └────────────────┘ │
                                              └────────────────────┘
```

**Discovery:** lspd writes `~/.factory/ide/<port>.lock` on startup — the same lock file format the VS Code/Cursor extension uses. Droid's `IdeContextManager` auto-discovers it. No environment variables to set.

**Write path:** Droid's built-in `fetchDiagnostics` calls lspd's `getIdeDiagnostics` MCP endpoint before and after each edit. Droid diffs the results and attaches new errors to the tool result.

**Read path:** A PostToolUse hook runs `lsp-read-hook`, which connects to lspd's Unix socket, peeks the diagnostic store, and injects a `<system-reminder>` via `hookSpecificOutput.additionalContext`.

**Coexistence:** When VS Code/Cursor is connected, Droid prefers the real IDE for writes (detected via `preferredIdeName` matching). lspd only handles writes when no IDE is present. The Read hook runs in all scenarios.

---

## Supported languages

| Language | Server | Extensions |
|---|---|---|
| TypeScript / JavaScript | `typescript-language-server` | `.ts` `.tsx` `.js` `.jsx` `.mts` `.cts` |
| Python | `pyright-langserver` | `.py` |
| Go | `gopls` | `.go` |
| Rust | `rust-analyzer` | `.rs` |
| C / C++ | `clangd` | `.c` `.cc` `.cpp` `.cxx` `.h` `.hpp` `.hxx` |

Language servers are spawned lazily on first file touch. Adding a language is a config entry:

```yaml
# ~/.factory/hooks/lsp/lspd.yaml
languages:
  ruby:
    command: solargraph
    args: [stdio]
    extensions: [.rb]
    root_markers: [Gemfile]
```

---

## Configuration

Config lives at `~/.factory/hooks/lsp/lspd.yaml`. Per-project overrides go in `<project>/.factory/lsp/lspd.yaml`.

```yaml
run_dir: ~/.factory/run/lspd
log_file: ~/.factory/logs/lspd/lspd.log
socket:
  path: ~/.factory/run/lspd/lspd.sock

policy:
  min_severity: warning          # drop info/hint
  max_diagnostics_per_file: 20
  max_diagnostics_per_turn: 50
  attach_code_actions: true      # show quick-fix previews with errors
  source_denylist:
    - eslint-plugin-import       # suppress noisy sources
```

---

## Multi-harness support

lspd's core is harness-agnostic. The diagnostic store, LSP pool, and MCP tools work the same regardless of which agent calls them.

| Harness | Write diagnostics | Read diagnostics | How it connects |
|---|---|---|---|
| **Droid** | Automatic (native `fetchDiagnostics` pipeline) | Automatic (PostToolUse Read hook) | Lock file auto-discovery |
| **Codex** | Model-initiated (MCP tool call + system prompt instruction) | PostToolUse hook on shell reads | MCP server in `config.toml` |
| **Claude Code** | Built-in (v2.0.74+ has native LSP) | PostToolUse Read hook adds value | Lock file or MCP server |
| **Any MCP client** | Via `getIdeDiagnostics` tool call | Via `getIdeDiagnostics` tool call | Connect to lspd's MCP endpoint |

---

## Competitive landscape

No existing project combines automatic push diagnostics, a policy layer, semantic navigation, and multi-harness integration.

| Capability | [Serena](https://github.com/oraios/serena) (22.8k stars) | [mcp-language-server](https://github.com/isaacphi/mcp-language-server) (1.5k) | [mcpls](https://github.com/bug-ops/mcpls) (32) | Claude Code native | **lspd** |
|---|---|---|---|---|---|
| Auto diagnostics after writes | ❌ | ❌ | ❌ | ✅ | ✅ |
| Auto diagnostics after reads | ❌ | ❌ | ❌ | ❌ | ✅ |
| Pull diagnostics on demand | ❌ | ✅ | ✅ | ✅ | ✅ |
| Policy layer (dedup/volume/severity) | ❌ | ❌ | ❌ | Partial | ✅ |
| Semantic navigation | ✅ (richer) | ✅ | ✅ | ✅ | ✅ |
| Multi-harness | ❌ | Agnostic | ❌ | Claude Code only | ✅ |
| Language count | 40+ | 5 | 30+ | Configurable | 5 (extensible) |

**Complements Serena:** Serena handles symbol navigation, lspd handles diagnostics. Run both — no conflict.

**Replaces mcp-language-server/cclsp** for the diagnostic use case — push instead of pull, with policy to prevent noise.

**Unnecessary with Claude Code native LSP** — Claude Code already has push diagnostics. lspd targets contexts where that isn't available.

See also: [Codex CLI issue #8745](https://github.com/openai/codex/issues/8745) — open request for this exact capability.

---

## CLI

| Command | Purpose |
|---|---|
| `lspd start [--foreground] [--config PATH]` | Start the daemon |
| `lspd stop [--config PATH]` | Stop gracefully |
| `lspd status [--json] [--config PATH]` | Show state |
| `lspd reload [--config PATH]` | Hot-reload config (also `kill -HUP $PID`) |
| `lspd ping [--config PATH]` | Liveness check |
| `lspd diag PATH` | Print diagnostics for a file |
| `lspd fix PATH:LINE` | Print code actions at a position |

---

## Development

```sh
go build ./...
go vet ./...
go test -race ./...
go test -race -tags integration ./test/integration/...  # requires language servers
```

---

## License

MIT
