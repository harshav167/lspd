# lspd

**Give your AI coding agent the same LSP feedback a human gets from their IDE — without needing an IDE.**

`lspd` is a lightweight Go daemon that runs alongside Droid and feeds it real-time compiler diagnostics and semantic code-navigation tools through the same MCP integration seam that VS Code uses. After every file write and every file read, the agent sees what broke — the same way you see squiggly red lines the moment you stop typing.

---

## Why not just use VS Code with the Droid extension?

You can. If you're already working inside VS Code, Cursor, or Windsurf with the Factory extension installed, you already have this capability. The extension connects to Droid over `FACTORY_VSCODE_MCP_PORT` and provides exactly the same diagnostic injection and IDE tools that lspd provides.

**lspd exists for every other context:**

| Scenario | VS Code extension | lspd |
|---|---|---|
| Working in a terminal (iTerm2, kitty, Warp, tmux) | Not available — no GUI IDE running | Works. One Go binary, no GUI |
| SSH into a remote server | Need VS Code Remote or tunneling | Works natively. Run lspd on the server, diagnostics flow |
| CI/CD pipelines and automated code generation | No IDE in CI | Works. Same daemon, same feedback loop |
| Docker containers / devcontainers without VS Code | Only if VS Code manages the container | Works. Install lspd in the container image |
| Low-resource machines (4GB RAM servers, Raspberry Pi) | VS Code + Electron = 500MB–2GB RAM | lspd = ~20MB Go binary + language server footprint |
| Headless droid exec / missions | Extension can't attach to headless runs | Works. Launcher wrapper sets the port before exec |
| Air-gapped / restricted environments | Need extension marketplace access | lspd has zero external dependencies beyond language servers |

**If you work exclusively inside VS Code, you don't need lspd.** The extension does everything lspd does. lspd is for the other 50% of the time when you're in a terminal, on a server, in CI, or anywhere a GUI IDE isn't practical.

---

## What it does

### 1. Diagnostic injection after every write

When Droid edits, creates, or patches a file, its built-in `fetchDiagnostics` pipeline calls lspd's `getIdeDiagnostics` MCP endpoint. lspd asks the running language server (gopls, pyright, typescript-language-server, rust-analyzer, or clangd) for the current diagnostics, diffs them against the pre-edit snapshot, and returns only the **new** errors and warnings. Droid formats them as a `<system-reminder>` block and attaches it to the tool result:

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

The agent sees this on its very next turn, in the same message as the edit result. No `go build`. No `tsc --noEmit`. No compile-error-patch-compile loop. The language server already knew about the error 200ms after the edit — lspd just makes sure the agent knows too.

### 2. Diagnostic injection after every read (the Read hook)

This is something **even VS Code's extension doesn't always do**. When the agent reads a file, a PostToolUse hook fires, queries lspd's diagnostic store, and injects any known diagnostics for that file as a system message. The agent sees the errors *before it starts planning changes* — proactive awareness, not reactive discovery.

This means: if you ask the agent to "read `src/api/users.ts` and tell me what's wrong," it gets the LSP diagnostics alongside the file contents, without needing to compile anything. The agent knows the file is broken before it writes a single line of code.

### 3. Semantic code-navigation tools

Because lspd is already running full LSP clients for each language, it exposes 10 semantic tools the model can call directly:

| Tool | What it does | Replaces |
|---|---|---|
| `lspDefinition` | Jump to the definition of a symbol | Grepping for `function foo` |
| `lspReferences` | Find every usage of a symbol across the project | `grep -r "foo" src/` |
| `lspHover` | Get the type signature and docs for a symbol | Reading the source to infer the type |
| `lspWorkspaceSymbol` | Fuzzy-search every symbol the language server knows | `grep -r` across the whole repo |
| `lspDocumentSymbol` | Get the hierarchical outline of a file | Reading the whole file to understand its structure |
| `lspCodeActions` | Get the language server's suggested fixes for an error | Guessing what import to add |
| `lspRename` | Prepare a safe cross-file rename | Manual search-and-replace |
| `lspFormat` | Format a file per the project's formatter config | Shelling out to `prettier`/`gofmt` |
| `lspCallHierarchy` | "Who calls this?" / "What does this call?" | Grepping for function names |
| `lspTypeHierarchy` | "What implements this interface?" / "What does this extend?" | Manually tracing inheritance |

These are exposed as first-class MCP tools with descriptions that teach the model when to prefer them over text search. The model reaches for `lspReferences` instead of `Grep` when doing impact analysis, because the description says "the language server filters out comments, strings, and unrelated same-name identifiers."

---

## The build-fix-build-fix problem

Without inline diagnostics, AI coding agents spend an enormous fraction of their tokens and wall-clock time in this loop:

1. Write a file
2. Run `go build` / `tsc` / `cargo build`
3. Get an error
4. Patch the file
5. Run the compiler again
6. Get a different error
7. Repeat 5–15 times

From a real GPT-5.4 session building this very project:

```
go build → undefined: protocol.TypeScript → patch → rebuild
         → undefined: protocol.Python → patch → rebuild
         → undefined: net → add import → rebuild
         → m.server.Shutdown returns 1 value → fix → rebuild
         → undefined: protocol.ErrUnknownProtocol → fix → rebuild
         → "strings" imported and not used → remove → rebuild
         → "go.lsp.dev/protocol" imported and not used → remove → rebuild
         → type mismatch []client.typeHierarchyItem → export + alias → rebuild
         → "go.lsp.dev/protocol" imported and not used (different file) → remove → rebuild
```

**Eleven compile cycles.** Each one a round-trip through the compiler. Each one a separate tool call. Each one polluting the model's context with error output.

With lspd, the same eleven errors surface as `<system-reminder>` blocks attached to the original edits — zero standalone compile cycles. The agent writes code the way you write code in an IDE: errors appear inline, you fix them while they're fresh, and you move on.

---

## Quick start

### Install

```sh
cd /path/to/droid-lsp
go build -o lspd ./cmd/lspd
go build -o lsp-read-hook ./cmd/lsp-read-hook
./scripts/install.sh
```

This installs:
- `~/.local/bin/lspd` — the daemon binary
- `~/.local/bin/lsp-read-hook` — the PostToolUse Read hook binary
- `~/.local/bin/droid-lsp` — the launcher wrapper
- `~/.local/bin/droid-lsp-settings.json` — process-local hook settings
- `~/.local/bin/droid-lsp-config.yaml` — dedicated daemon config

### Use

Instead of running `droid`, run:

```sh
droid-lsp
```

That's it. The wrapper:
1. Starts (or reuses) the lspd daemon
2. Exports `FACTORY_VSCODE_MCP_PORT` pointing at it
3. Injects hook settings for Read-time diagnostics and SessionEnd cleanup
4. Execs the real `droid` binary

You're now in a normal Droid session with full LSP diagnostics and semantic tools.

### Verify it works

In a droid-lsp session, create a Go file with intentional errors:

```go
package main

import (
    "fmt"
    "strings"
)

func broken() string {
    return 123
}

func main() {
    fmt.Println(missingName)
}
```

After the write, you should see a `<system-reminder>` with three errors (unused import, type mismatch, undefined name). After reading the file, the Read hook should surface the same diagnostics. If both appear, lspd is working.

---

## How it works (architecture)

```
  Droid process                              lspd daemon
  ┌──────────────┐                          ┌──────────────────────┐
  │ Edit/Create   │──── getIdeDiagnostics ──►│ MCP server           │
  │ (native path) │◄── {diagnostics:[...]} ──│ (StreamableHTTP)     │
  │               │                          │                      │
  │ Read          │                          │ ┌──────────────────┐ │
  │ (hook path)   │──── socket peek ────────►│ │ Diagnostic store │ │
  │               │◄── <system-reminder> ────│ │ (per-URI,        │ │
  │               │                          │ │  versioned)      │ │
  │ lspDefinition │──── MCP tool call ──────►│ └────────┬─────────┘ │
  │ lspReferences │◄── {definitions:[...]} ──│          │           │
  │ lspHover      │                          │     publishDiag.     │
  │ ...           │                          │          │           │
  └──────────────┘                          │ ┌────────┴─────────┐ │
                                            │ │ LSP client pool  │ │
                                            │ │ ts│py│go│rs│cpp  │ │
                                            │ └──┬──┬──┬──┬──┬───┘ │
                                            └────┼──┼──┼──┼──┼────┘
                                                 │  │  │  │  │
                                            tsserver pyright gopls
                                            (stdio)  (stdio) (stdio)
```

**Write path (native):** Droid's `edit-cli.ts` calls `fetchDiagnostics(ideClient, filePath)` before and after every edit. `ideClient.callTool('getIdeDiagnostics', {uri})` goes to lspd over MCP. lspd asks the language server, applies policy (dedup, volume caps, severity filter), and returns. Droid diffs before/after and formats the `<system-reminder>`. No hook needed — this is Droid's built-in pipeline with lspd as the backend.

**Read path (hook):** Droid's `read-cli.ts` doesn't call `fetchDiagnostics`. The PostToolUse Read hook fills the gap: `lsp-read-hook` connects to lspd's Unix socket, peeks the diagnostic store for the read file, formats a `<system-reminder>`, and returns it as `hookSpecificOutput.additionalContext`. Droid injects it as a System message before the agent's next turn.

**Navigation path (MCP tools):** The model calls `lspDefinition`, `lspReferences`, etc. as native MCP tool calls. lspd routes to the right language server, issues the LSP request, and returns LLM-friendly JSON with 1-indexed line numbers and source-line previews.

---

## Supported languages

| Language | Server | Extensions |
|---|---|---|
| TypeScript / JavaScript | `typescript-language-server` | `.ts`, `.tsx`, `.js`, `.jsx`, `.mts`, `.cts` |
| Python | `pyright-langserver` | `.py` |
| Go | `gopls` | `.go` |
| Rust | `rust-analyzer` | `.rs` |
| C / C++ | `clangd` | `.c`, `.cc`, `.cpp`, `.cxx`, `.h`, `.hpp`, `.hxx` |

Language servers are spawned lazily on first file touch. Adding a new language is a YAML config entry — no code change needed. See `examples/lspd.yaml` for the full config schema.

---

## Configuration

Default config lives at `~/.local/bin/droid-lsp-config.yaml` (installed by `install.sh`). Per-project overrides go in `<project>/.factory/lsp/lspd.yaml`.

Key fields:

```yaml
# Runtime paths
run_dir: ~/.factory/run/droid-lsp
log_file: ~/.factory/logs/droid-lsp/lspd.log
socket:
  path: ~/.factory/run/droid-lsp/lspd.sock

# Policy (controls what diagnostics the agent sees)
policy:
  min_severity: warning        # drop info/hint by default
  max_diagnostics_per_file: 20
  max_diagnostics_per_turn: 50
  dedupe_scope: session        # same error shown once per session
  attach_code_actions: true    # show quick-fix previews alongside errors
  source_denylist:
    - eslint-plugin-import     # known noisy source

# Per-language overrides
languages:
  python:
    command: basedpyright-langserver  # swap pyright for basedpyright
    args: [--stdio]
```

---

## When to use lspd vs. VS Code extension

| | lspd | VS Code + Factory extension |
|---|---|---|
| Best for | Terminal, SSH, CI, headless, resource-constrained | Interactive development with a GUI |
| Diagnostic injection | ✅ after every Edit/Create/Read | ✅ after every Edit/Create (Read may vary) |
| Semantic navigation tools | ✅ 10 MCP tools | ✅ (via extension's IDE integration) |
| Resource usage | ~20MB binary + language servers | VS Code (~500MB) + extension + language servers |
| Setup | `./scripts/install.sh` + run `droid-lsp` | Install VS Code + install extension |
| Works without GUI | ✅ | ❌ |
| Works in CI/automation | ✅ | ❌ (no interactive GUI) |
| Works over SSH | ✅ | Only with VS Code Remote |
| Per-project language server config | YAML file | VS Code settings.json |

**Use VS Code** when you want the full IDE experience alongside Droid.
**Use lspd** when you want Droid to have IDE-grade feedback without the IDE.

---

## CLI reference

| Command | Purpose |
|---|---|
| `lspd start [--foreground] [--config PATH]` | Start the daemon (daemonizes unless `--foreground`) |
| `lspd stop [--config PATH]` | Stop the daemon gracefully |
| `lspd status [--json] [--config PATH]` | Show daemon state, language servers, sessions |
| `lspd reload [--config PATH]` | Hot-reload config (also triggered by `kill -HUP $PID`) |
| `lspd ping [--config PATH]` | Liveness check |
| `lspd diag PATH` | Print current diagnostics for a file (debugging) |
| `lspd fix PATH:LINE` | Print available code actions at a position (debugging) |
| `lspd logs [--follow]` | Tail the structured log file |

---

## Development

```sh
# Build
go build ./...

# Test (unit + e2e)
go test -race ./...

# Test (integration — requires language servers installed)
go test -race -tags integration ./test/integration/...

# Lint
go vet ./...
```

See [PLAN.md](./PLAN.md) for the full architecture, implementation plan, and design decisions.

---

## Why this project exists — the competitive landscape

There are roughly a dozen projects in the "LSP for AI coding agents" space. None of them do what this project does. The field splits into two camps, and this project bridges the gap between them.

### Camp 1: Navigation only — no diagnostics

| Project | Stars | Languages | What it does | What it doesn't do |
|---|---|---|---|---|
| [Serena](https://github.com/oraios/serena) | 22.8k | 40+ | Symbol-level navigation and editing via MCP — find symbols, references, rename, replace symbol body, call hierarchy. The most popular tool in this space by far. | **Zero diagnostic capability.** Every language server wrapper deliberately discards `publishDiagnostics` notifications. Agents using Serena can navigate code semantically but have no idea if it compiles. |

Serena is excellent at what it does. If you need symbol-level code navigation for your agent, use Serena. But Serena won't tell your agent that it just introduced a type error, a missing import, or an undefined variable — and that's the class of feedback that eliminates the build-fix-build-fix loop.

### Camp 2: Pull diagnostics — agent must ask

| Project | Stars | Languages | Diagnostics | Navigation | Key limitation |
|---|---|---|---|---|---|
| [mcp-language-server](https://github.com/isaacphi/mcp-language-server) | 1.5k | Go, Rust, Python, TS, C/C++ | Pull only — `diagnostics` tool | definition, references, hover, rename | Agent must explicitly call the diagnostic tool after every edit. If it forgets, errors go unnoticed. One LSP server per run. |
| [cclsp](https://github.com/ktnyt/cclsp) | 609 | TS, Go, Rust, Python, C++, Java, Ruby, PHP | Pull only — `get_diagnostics` tool | find_definition, find_references, rename | Same — agent must ask. No dedup, no volume caps, no severity filtering. |
| [mcpls](https://github.com/bug-ops/mcpls) | 32 | 30+ (Rust, Python, TS, Go, C/C++, Java, Zig) | Both pull and cached — `get_diagnostics` + `get_cached_diagnostics` | hover, definition, references, completions, rename, call hierarchy, format | Closest architectural twin to this project. Rust binary, multiple concurrent LSP clients. But no harness integration — the agent still has to call the tool. No automatic injection. No policy layer. |
| [lsp-mcp](https://github.com/Tritlo/lsp-mcp) | 119 | Haskell, TS, any LSP via config | Subscription-based — `lsp-diagnostics://` resource | get_info_on_location, completions, code_actions | Requires explicit `open_document` before diagnostics work. Haskell-first audience. |

These tools give agents access to diagnostics, but the agent has to remember to ask after every edit. If it doesn't — and models frequently don't — errors accumulate silently until the next `go build` or `tsc` invocation, and you're back in the compile-error-patch-compile loop.

### What's missing from both camps

No existing project combines:

1. **Automatic push-style diagnostic injection** — errors surface after every write and every read without the agent asking
2. **A policy layer** — session-scoped dedup (same error shown once), per-file volume caps (max 20), per-turn volume caps (max 50), severity filtering (drop info/hint), source denylist (suppress noisy linters)
3. **Semantic navigation tools** — definition, references, hover, workspace symbol, document symbol, code actions, rename, format, call hierarchy, type hierarchy
4. **Multi-harness integration** — works with Droid (native IDE seam + hooks), Codex (hooks + MCP), and any MCP-compatible agent

This project fills that gap. It's a single daemon that runs your language servers, collects their diagnostics, applies policy to prevent context pollution, and delivers errors to the agent before the agent's next decision — the same feedback loop a human gets from their IDE, running headlessly for any AI coding agent.

### Where this fits alongside existing tools

**This project complements Serena.** If you already use Serena for symbol navigation, this project adds the diagnostic half that Serena deliberately doesn't provide. Run both — Serena handles your `find_symbol` and `rename_symbol` calls, this project handles your "what errors did I just introduce?" feedback. They use separate LSP server instances so there's no conflict.

**This project replaces mcp-language-server / cclsp for the diagnostic use case** — same capability but automatic (push) instead of manual (pull), with the policy layer that prevents diagnostic noise from flooding the agent's context in large codebases.

**This project is unnecessary if you use Claude Code with its native LSP** (v2.0.74+). Claude Code already has built-in push diagnostics after writes. This project targets the contexts where Claude Code's native LSP isn't available: terminal sessions, Droid, Codex, SSH, CI/CD, headless automation, or any agent harness that doesn't have its own LSP integration.

### The demand is real

- [Codex CLI issue #8745](https://github.com/openai/codex/issues/8745) (January 2026) — open request for exactly this capability: LSP integration with a fix-loop. Three months, no implementation.
- [Amazon Kiro](https://kiro.dev/blog/empowering-kiro-with-ide-diagnostics/) reported 29% reduction in command executions after adding IDE diagnostics to their agentic workflow — but Kiro is a proprietary IDE, not an open tool.
- Every coding agent that runs in a terminal without an IDE faces the same build-fix-build-fix loop this project eliminates.

### Comparison matrix

| Capability | Serena | mcp-language-server | cclsp | mcpls | Claude Code native | **This project** |
|---|---|---|---|---|---|---|
| Push diagnostics (auto after writes) | ❌ | ❌ | ❌ | ❌ | ✅ | ✅ |
| Push diagnostics (auto after reads) | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Pull diagnostics (on demand) | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Policy layer (dedup/volume/severity) | ❌ | ❌ | ❌ | ❌ | Partial | ✅ |
| Code action preview with errors | ❌ | ❌ | ❌ | Partial | ❌ | ✅ |
| Semantic navigation (def/refs/hover) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Symbol-level editing (replace body) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Multi-harness (Droid/Codex/Claude Code) | ❌ | Agnostic but no hooks | ❌ | ❌ | Claude Code only | ✅ |
| Works without GUI IDE | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Language count | 40+ | 5 | 8 | 30+ | Configurable | 5 (extensible via YAML) |
| Stars | 22.8k | 1.5k | 609 | 32 | N/A (built-in) | New |

---

## License

MIT
