# lspd

[![CI](https://github.com/harshav167/lspd/actions/workflows/ci.yml/badge.svg)](https://github.com/harshav167/lspd/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/harshav167/lspd)](https://github.com/harshav167/lspd/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Give your AI coding agent the same red-squiggly-line feedback a human gets from their IDE — without needing an IDE.**

Built primarily for [Factory Droid](https://factory.ai), with [Codex CLI](https://github.com/openai/codex) support as the next priority. The core is harness-agnostic — it works with any agent harness that supports post-tool-use hooks.

<!-- TODO: Add a GIF here showing: agent edits a file → diagnostics appear inline on the next turn -->

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/install.sh | sh
```

That's it. Start lspd for your current coding session, then run your agent normally:

```sh
lspd start
droid
```

The installer:
1. Downloads `lspd` and `lsp-read-hook` binaries for your platform
2. Writes daemon config to `~/.factory/hooks/lsp/lspd.yaml`
3. Merges the Read diagnostic hook and SessionEnd cleanup hook into your `~/.factory/settings.json` (non-destructive — your existing hooks are preserved)
4. Leaves your regular `droid` command untouched

No wrapper script. No alias. No environment variables to set. `lspd start` starts the daemon for the current work session, registers itself via a lock file at `~/.factory/ide/`, and Droid auto-discovers it — the same mechanism the VS Code extension uses.

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

No `go build`. No `tsc --noEmit`. No compile-error-patch-compile loop. The language server already knew about the error 200ms after the edit — lspd just makes sure the agent knows too.

### 2. Diagnostics after every read

When the agent reads a file, a PostToolUse hook queries lspd and injects known diagnostics as a system message. The agent sees errors *before it starts planning changes* — proactive awareness, not reactive discovery.

If you ask the agent to "read `src/api/users.ts` and tell me what's wrong," it gets the LSP diagnostics alongside the file contents, without needing to compile anything. The agent knows the file is broken before it writes a single line of code.

### 3. Semantic navigation tools

Because lspd already runs language servers, it also exposes 10 LSP-powered tools the model can call directly:

| Tool | What it does | What it replaces |
|---|---|---|
| `lspDefinition` | Jump to the definition of a symbol | Grepping for `function foo` |
| `lspReferences` | Find every usage of a symbol across the project | `grep -r "foo" src/` |
| `lspHover` | Get the type signature and docs for a symbol | Reading source to figure out a type |
| `lspWorkspaceSymbol` | Fuzzy-search every symbol the language server knows | Searching the repo for a symbol name |
| `lspDocumentSymbol` | Get the hierarchical outline of a file | Reading a whole file to understand its structure |
| `lspCodeActions` | Get the language server's suggested fixes for an error | Guessing which import to add |
| `lspRename` | Prepare a safe cross-file rename | Find-and-replace across files |
| `lspFormat` | Format a file per the project's formatter config | Shelling out to `prettier` or `gofmt` |
| `lspCallHierarchy` | "Who calls this?" / "What does this call?" | Grepping for function names |
| `lspTypeHierarchy` | "What implements this interface?" / "What does this extend?" | Manually tracing inheritance |

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
         → type mismatch []client.typeHierarchyItem → export + alias → rebuild
         → "go.lsp.dev/protocol" imported and not used (different file) → remove → rebuild
```

With lspd: zero compile cycles. Each error surfaces as a `<system-reminder>` attached to the edit that caused it.

---

## How it works

lspd is a Go daemon that manages language server subprocesses (gopls, pyright, typescript-language-server, rust-analyzer, clangd) and injects their diagnostics into the agent's context via hooks and Droid's native IDE integration seam.

```
  Agent harness (Droid / Codex / etc.)        lspd daemon
  ┌──────────────────┐                       ┌────────────────────┐
  │ Write/Edit/Create │── getIdeDiagnostics ─►│ IDE bridge         │
  │ (native pipeline) │◄─ {diagnostics:[]} ──│ (StreamableHTTP)   │
  │                   │                       │                    │
  │ Read              │                       │ ┌────────────────┐ │
  │ (PostToolUse hook)│── socket peek ───────►│ │ Diagnostic     │ │
  │                   │◄─ <system-reminder> ──│ │ store          │ │
  │                   │                       │ └───────┬────────┘ │
  │ lspDefinition     │── tool call ─────────►│         │          │
  │ lspReferences     │◄─ {locations:[]} ────│    publishDiag.    │
  │ lspHover, etc.    │                       │         │          │
  └──────────────────┘                       │ ┌───────┴────────┐ │
                                              │ │ LSP pool       │ │
                                              │ │ ts py go rs cc │ │
                                              │ └────────────────┘ │
                                              └────────────────────┘
```

**Discovery:** lspd writes `~/.factory/ide/<port>.lock` on startup — the same lock file format the VS Code/Cursor extension uses. Droid's `IdeContextManager` auto-discovers it. No environment variables to set.

**Write path:** Droid's built-in `fetchDiagnostics` calls lspd's `getIdeDiagnostics` endpoint before and after each edit. Droid diffs the results and attaches new errors to the tool result. No hooks needed — lspd plugs into the same slot the VS Code extension uses.

**Read path:** A PostToolUse hook runs `lsp-read-hook`, which connects to lspd's Unix socket, peeks the diagnostic store, and injects a `<system-reminder>` via `hookSpecificOutput.additionalContext`. The agent sees problems before it starts editing.

**Coexistence:** When VS Code/Cursor is connected, Droid prefers the real IDE for writes (detected via `preferredIdeName` matching). lspd only handles writes when no IDE is present. The Read hook runs in all scenarios.

---

## Supported languages

| Language | Server | Extensions |
|---|---|---|
| TypeScript / JavaScript* | `typescript-language-server` | `.ts` `.tsx` `.js` `.jsx` `.mts` `.cts` |
| Python | `pyright-langserver` | `.py` |
| Go | `gopls` | `.go` |
| Rust | `rust-analyzer` | `.rs` |
| C / C++ | `clangd` | `.c` `.cc` `.cpp` `.cxx` `.h` `.hpp` `.hxx` |

\* TypeScript has a known issue with `getIdeDiagnostics` timing out on cold start — see [Known issues](#known-issues).

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

## Install

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/install.sh | sh
```

That's it. Start lspd explicitly for the current work session:

```sh
lspd start
droid
```

The installer:
1. Downloads `lspd` and `lsp-read-hook` binaries for your platform
2. Writes daemon config to `~/.factory/hooks/lsp/lspd.yaml`
3. Merges only the `Read` diagnostic hook plus `SessionEnd` cleanup hook into your `~/.factory/settings.json`
4. Keeps your normal `droid` binary untouched

`lspd` is session-scoped by default. It shuts itself down after its idle timeout or when you run:

```sh
lspd stop
```

### Update

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/install.sh | sh
```

Same command. It downloads the latest binaries and overwrites the old ones. Config is preserved.

### Uninstall

```sh
curl -fsSL https://github.com/harshav167/lspd/releases/latest/download/uninstall.sh | sh
```

Removes binaries, hooks, optional convenience wrapper, and runtime state. Your config at `~/.factory/hooks/lsp/lspd.yaml` is kept unless you pass `--purge`.

---

## Configuration

Default config lives at `~/.factory/hooks/lsp/lspd.yaml`. Per-project overrides go in `<project>/.factory/lsp/lspd.yaml`.

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

## Harness support

| Harness | Write diagnostics | Read diagnostics | Integration | Status |
|---|---|---|---|---|
| **[Droid](https://factory.ai)** | Automatic — native `fetchDiagnostics` pipeline | Automatic — PostToolUse Read hook | Lock file auto-discovery + hooks | **Working** |
| **[Codex](https://github.com/openai/codex)** | Hook + system prompt instruction | PostToolUse hook (shell reads only — `apply_patch` doesn't fire hooks) | Hooks in `~/.codex/hooks.json` | WIP |
| **[Claude Code](https://claude.ai/code)** | Not needed — Claude Code v2.0.74+ has native LSP push diagnostics | Read hook can add value | N/A | Not needed |
| **[Cline](https://github.com/cline/cline) / [Aider](https://github.com/paul-gauthier/aider) / others** | Via hooks or tool calls | Via hooks or tool calls | Register lspd as tool server | Untested |

---

## Competitive landscape

No existing project combines automatic push diagnostics, a policy layer, semantic navigation, and multi-harness integration.

| Capability | [Serena](https://github.com/oraios/serena) (22.8k stars) | [mcp-language-server](https://github.com/isaacphi/mcp-language-server) (1.5k) | [mcpls](https://github.com/bug-ops/mcpls) (32) | Claude Code native | **lspd** |
|---|---|---|---|---|---|
| Auto diagnostics after writes | ❌ | ❌ | ❌ | ✅ | ✅ |
| Auto diagnostics after reads | ❌ | ❌ | ❌ | ❌ | ✅ |
| Pull diagnostics on demand | ❌ | ✅ | ✅ | ✅ | ✅ |
| Policy layer (dedup/volume/severity) | ❌ | ❌ | ❌ | Partial | ✅ |
| Code action preview with errors | ❌ | ❌ | Partial | ❌ | ✅ |
| Semantic navigation (def/refs/hover) | ✅ (richer) | ✅ | ✅ | ✅ | ✅ |
| Symbol-level editing (replace body) | ✅ | ❌ | ❌ | ❌ | ❌ |
| Multi-harness | ❌ | Agnostic but no hooks | ❌ | Claude Code only | ✅ |
| Language count | 40+ | 5 | 30+ | Configurable | 5 (extensible via YAML) |

**Complements Serena:** Serena handles symbol navigation, lspd handles diagnostics. Run both — no conflict.

**Replaces mcp-language-server / cclsp** for the diagnostic use case — push instead of pull, with policy to prevent noise.

**Unnecessary with Claude Code native LSP** — Claude Code already has push diagnostics. lspd targets contexts where that isn't available.

See also: [Codex CLI issue #8745](https://github.com/openai/codex/issues/8745) — open request for this exact capability.

---

## CLI

```
lspd start [--foreground] [--config PATH]    Start the daemon
lspd stop [--config PATH]                    Stop gracefully
lspd status [--json] [--config PATH]         Show state
lspd reload [--config PATH]                  Hot-reload config
lspd ping [--config PATH]                    Liveness check
lspd diag PATH                               Print diagnostics for a file
lspd fix PATH:LINE                           Print code actions at a position
```

---

## Development

```sh
go build ./...
go vet ./...
go test -race ./...
act push              # run CI locally before pushing
```

---

## Known issues

### TypeScript `getIdeDiagnostics` timeout

The model-callable `getIdeDiagnostics` MCP tool currently **times out for TypeScript files** on projects with dependencies (`tsconfig.json` + `node_modules`). This is a cold-start issue with `typescript-language-server` / `tsserver` — the server spawns successfully, `didOpen` fires, but `textDocument/publishDiagnostics` notifications don't arrive within the wait timeout.

**What works:**
- Write-time diagnostics via Droid's native `fetchDiagnostics` pipeline (the automatic `<system-reminder>` after Edit/Create/ApplyPatch)
- Read-time diagnostics via the PostToolUse Read hook (the automatic `<system-reminder>` after Read)
- Other languages — Go (`gopls`), Python (`pyright`), Rust (`rust-analyzer`), C/C++ (`clangd`) all return diagnostics correctly from `getIdeDiagnostics`

**What doesn't work:**
- Explicit model calls to `getIdeDiagnostics` on `.ts` / `.tsx` files return empty or time out after 3 seconds

**Workaround:** Use the automatic injection paths (write/read hooks). They work for TypeScript because they query the diagnostic store directly after the LSP server has published, rather than waiting synchronously on a specific version.

**Fix in progress.** Tracking as [#1](https://github.com/harshav167/lspd/issues/1). Likely requires either a longer cold-start timeout, `workspace/configuration` request handling, or switching the default TS server to `vtsls`.

---

## License

MIT
