# `lspd` — Droid LSP Bridge: Architecture & Implementation Plan

> **Decision (2026-04-10):** Build a comprehensive, feature-rich Go daemon that impersonates the Droid VS Code IDE extension over `FACTORY_VSCODE_MCP_PORT`. It handles both diagnostic push delivery and semantic code navigation as first-class native IDE tools. Serena is explicitly out of scope of this project — it remains an optional MCP plugin the user may add separately; no code path in `lspd` is aware of it or depends on it.

> **Rationale:** Droid's IDE integration seam (`VSCodeIdeClient` → MCP over StreamableHTTP → tool result injection) is the *native* extension point the Droid maintainers support. Any tool exposed there gets first-class treatment: diagnostics ride along with the tool result as `<system-reminder>` blocks, and MCP tools from that endpoint are treated as IDE-provided tools rather than third-party plugins. Constraining ourselves to only the diagnostic subset would throw away the rest of that seam for no reason. If we're already impersonating an IDE, we should impersonate it fully.

> **Goal:** After every Droid Read, Edit, Create, Write, and apply-patch tool call, the relevant language server's diagnostics for that file are injected into the agent's next LLM turn before the agent takes any further action. Additionally, the model gains first-class access to semantic code-navigation tools (definition, references, hover, workspace symbol, document symbol, code actions, rename, format, call hierarchy, type hierarchy) via the same native IDE seam.

---

## Why this exists

AI coding agents today spend an enormous fraction of their tokens, wall-clock time, and context window in a tight, miserable loop that anyone who has watched a session unfold will recognize:

1. Write a file with a tool call.
2. Run `go build ./...` (or `tsc`, or `cargo build`, or `pyright`, or whatever compiler fits).
3. Get an error: undefined symbol, wrong type, missing import, unused import.
4. Read the error.
5. Patch the file to fix that one error.
6. Run the compiler again.
7. Get a different error — maybe caused by the patch, maybe latent from a different file.
8. Patch.
9. Compile.
10. Repeat five to fifteen times per file until the whole package builds.

Every iteration of this loop burns tool calls, LLM tokens, developer patience, and model context window. A greenfield project of ~7,000 LOC can easily spend 40% of its build session stuck here, compiling, parsing errors, patching, compiling, parsing different errors, patching, and so on. By the time the compile finally goes green, the agent's context is half-full of stale error messages and the user has lost track of what the current state even is. This is the build-fix-build-fix treadmill, and it's the single biggest tax on multi-thousand-LOC agent builds.

### A real example

From an observed GPT-5.4 session attempting this very project (recorded during early lspd bootstrap), roughly five minutes of wall-clock time spent in the loop:

1. `go build ./...` → `internal/config/config.go:142: undefined: protocol.TypeScript` → patch to `protocol.LanguageIdentifier("typescript")` → rebuild
2. → `internal/config/config.go:156: undefined: protocol.Python` → patch → rebuild
3. → `internal/config/config.go:169: undefined: protocol.Go` → patch → rebuild
4. → `cmd/lsp-read-hook/main.go:32: undefined: net` → add the `"net"` import → rebuild
5. → `internal/lsp/client/manager.go:92: assignment mismatch: 2 variables but m.server.Shutdown returns 1 value` → change `_, _ =` to `_ =` → rebuild
6. → `internal/mcp/tools/compat/diagnostics.go:73: undefined: protocol.ErrUnknownProtocol` → swap for `url.InvalidHostError` → rebuild
7. → `internal/mcp/tools/nav/type_hierarchy.go:17: undefined: typeHierarchyItem` → define a local struct → rebuild
8. → `"strings" imported and not used` → remove the import → rebuild
9. → `"go.lsp.dev/protocol" imported and not used` → remove the import → rebuild
10. → `cannot use items (variable of type []client.typeHierarchyItem) as []typeHierarchyItem value in struct literal` → export the type from the client package and alias it in nav → rebuild
11. → `"go.lsp.dev/protocol" imported and not used` (in a different file this time) → remove → rebuild

Eleven compile cycles. Each one a round-trip of the entire package compiler against the disk state. Each one a separate tool call. Each one a fresh read of error output back into the model's context window. Most of these errors were visible to the language server the moment the agent's `ApplyPatch` tool wrote the file — `gopls`, `pyright`, `typescript-language-server`, `rust-analyzer`, and `clangd` all emit `publishDiagnostics` notifications within 100–500 ms of a `didChange` event. The agent simply had no way to see them, because nothing was forwarding those notifications from the language server back to the agent's next turn.

The `"imported and not used"` errors are especially telling. `gopls` reports these within one frame of a `didChange` — it's the canonical fast-feedback case. An agent with inline LSP feedback would never write an unused import in the first place, because the warning would land on the same turn as the edit, and the agent would strip the import before moving on to the next file. An agent without inline feedback writes the import, moves on, writes several more files, runs `go build` hundreds of milliseconds later, gets a wall of unused-import errors, parses them, and patches each one. Every unused import is three-to-five wasted tool calls that a real IDE user never pays.

### What lspd changes

With `lspd` running as a sidecar and impersonating droid's VS Code IDE integration over `FACTORY_VSCODE_MCP_PORT`, every `Edit` / `Create` / `ApplyPatch` tool call passes through droid's existing `fetchDiagnostics → compareDiagnostics → formatDiagnosticsForSystemReminder` pipeline (see §2.2). The pipeline fetches live LSP diagnostics for the edited file, diffs them against the pre-edit snapshot, and attaches any *new* errors or warnings to the tool result as a `<system-reminder>` block. The agent sees the error on its very next turn, **in the same message as the edit result**, before it decides what to do next. Reads get the same treatment via a PostToolUse Read hook (§11.3) that peeks the daemon's diagnostic store and injects current diagnostics into the Read's system-message payload.

That means the eleven-cycle loop above collapses into roughly this:

1. `ApplyPatch` to write `internal/config/config.go` → response includes `<system-reminder>: "Line 142: undefined: protocol.TypeScript (gopls)"` plus the same for lines 156 and 169 → agent fixes all three in the same file with the next `ApplyPatch` without ever running `go build`.
2. `ApplyPatch` to write `internal/lsp/client/manager.go` → response includes `<system-reminder>: "Line 92: assignment mismatch: 2 variables but m.server.Shutdown returns 1 value"` → agent fixes it inline before moving on.
3. `ApplyPatch` to write `internal/mcp/tools/compat/diagnostics.go` → response includes `<system-reminder>: "Line 73: undefined: protocol.ErrUnknownProtocol"` → fix inline.
4. `ApplyPatch` to write `internal/mcp/tools/nav/common.go` → response includes `<system-reminder>: "\"strings\" imported and not used"` → agent removes the import in the same turn it was about to use it for something else.

No intermediate `go build` calls. No stale error parsing. No guessing whether the last fix actually worked. The agent writes code the way a human developer with a modern IDE writes code: type-checks and lints appear inline as you edit, and you react to them immediately while the context is still fresh and the edit is still in working memory. For the eleven errors in the trace above, an lspd-enabled session produces eleven `<system-reminder>` blocks attached to the relevant `ApplyPatch` results, with zero standalone compile cycles between them. The token and tool-call savings are substantial. The quality improvement is even larger, because the agent sees each error in the immediate context of the edit that caused it, not half an hour later after six other edits have piled up on top.

### Why this is the right design

Claude Code's LSP attachment pipeline does exactly this for VS Code users today (see §2.1 for the architecture Claude Code's minified `cli.js` exposes). Droid's existing IDE integration does exactly this when droid runs inside VS Code, Cursor, Windsurf, or JetBrains (see §2.2 for the `fetchDiagnostics`/`compareDiagnostics`/`formatDiagnosticsForSystemReminder` pipeline droid ships). `lspd` extends that behavior to every droid session on every machine, including headless terminal sessions, CI runs, SSH workflows, remote container dev environments, and any other context where a full-fat GUI IDE isn't running. It reuses droid's existing `fetchDiagnostics` pipeline rather than layering a new mechanism on top, and it exposes the language server's other semantic capabilities (`lspDefinition`, `lspReferences`, `lspHover`, `lspWorkspaceSymbol`, `lspDocumentSymbol`, `lspCodeActions`, `lspRename`, `lspFormat`, `lspCallHierarchy`, `lspTypeHierarchy`) as first-class MCP tools alongside the diagnostic pipe (§6.3). One sidecar, one config, every language the user has a language server for.

The killer feature is not "the model can call `lspDefinition` instead of `grep`." That's a nice tier-two improvement, but it's not what justifies the project. The killer feature is: **after every file write and every file read, the agent sees what broke**, the same way a human developer sees squiggly red lines the moment they stop typing. The collapse of the build-fix-build-fix loop into a single fluent write-write-write-done flow is what makes multi-thousand-LOC one-shot agent builds viable in the first place. Without it, agents spend their sessions arguing with the compiler one error at a time. With it, they write code and the errors come to them, in the same turn, in time to act on without round-tripping through a separate compile command.

The GPT-5.4 trace above was recorded while bootstrapping lspd itself. The irony is load-bearing: the very project that eliminates the build-fix loop was being built inside the build-fix loop, because lspd wasn't wired up yet. Once lspd is live and the launcher wrapper sets `FACTORY_VSCODE_MCP_PORT` before `exec droid`, every subsequent droid session builds any project — including future lspd refactors — without the treadmill. Bootstrap once, collect the compounding benefit forever after.

---

## Table of Contents

- [Why this exists](#why-this-exists)

1. [Decision & Goal](#1-decision--goal)
2. [Background](#2-background)
   - 2.1 [How Claude Code delivers LSP diagnostics](#21-how-claude-code-delivers-lsp-diagnostics)
   - 2.2 [Droid's existing IDE integration](#22-droids-existing-ide-integration)
   - 2.3 [Droid's hook system](#23-droids-hook-system)
   - 2.4 [LSP fundamentals we rely on](#24-lsp-fundamentals-we-rely-on)
3. [Architecture](#3-architecture)
4. [The daemon: `lspd`](#4-the-daemon-lspd)
5. [LSP orchestration layer](#5-lsp-orchestration-layer)
6. [MCP server layer](#6-mcp-server-layer)
7. [Unix socket server](#7-unix-socket-server)
8. [Policy layer](#8-policy-layer)
9. [File watcher](#9-file-watcher)
10. [Configuration](#10-configuration)
11. [Droid integration](#11-droid-integration)
12. [Data flow walkthroughs](#12-data-flow-walkthroughs)
13. [Observability](#13-observability)
14. [Testing strategy](#14-testing-strategy)
15. [File structure & LOC budget](#15-file-structure--loc-budget)
16. [Rollout plan](#16-rollout-plan)
17. [Work unit decomposition](#17-work-unit-decomposition)
18. [Verification steps before build](#18-verification-steps-before-build)
- [Appendix A: Source references](#appendix-a-source-references)
- [Appendix B: LSP Diagnostic JSON shape](#appendix-b-lsp-diagnostic-json-shape)
- [Appendix C: `mcp-go` API sketch](#appendix-c-mcp-go-api-sketch)
- [Appendix D: `go.lsp.dev` API sketch](#appendix-d-golspdev-api-sketch)

---

## 1. Decision & Goal

> **SCOPE LOCK — READ THIS FIRST.** This is a one-shot build. Every file in §15, every MCP tool in §6, every work unit in §17 (39 total), every success criterion in §1.3 must be implemented before the work is done. Do not propose an "MVP", a "solid foundation", a "phase 1 only", a "minimal viable version", a "skeleton", or placeholder implementations of any tool listed in §6.3. Do not stop at the foundation sub-pass thinking someone else will finish the rest — you are the someone else. If you catch yourself thinking "I could skip X for now", "implementing the whole project in one go probably isn't what's meant", "I'll aim for something partially functional", or "the foundation is enough to show progress", STOP and re-read this paragraph. The plan is the pruned spec; do not re-prune it. If you are a solo implementer reading this, also read `SOLO_IMPL_CONTRACT.md` at the repo root before writing any code.

### 1.1 Explicit goal

After every Droid tool call that reads or modifies a file on disk — specifically `Read`, `Edit`, `Create`, `Write`, and `apply-patch` — the language server's current diagnostics for that file must be injected into the agent's next LLM turn before the agent is allowed to act on the tool result. "Before the agent is allowed to act" means synchronously, in the same turn, in the same conversation, not in a later turn or via a later tool call.

Additionally, the model gains a set of semantic code-navigation tools as first-class IDE-native tools, reachable via the same integration seam Droid already uses for VS Code. These tools are: `lspDefinition`, `lspReferences`, `lspHover`, `lspWorkspaceSymbol`, `lspDocumentSymbol`, `lspCodeActions`, `lspRename`, `lspFormat`, `lspCallHierarchy`, and `lspTypeHierarchy`. They replace grep-based navigation for semantic queries where the LSP has a more accurate answer.

### 1.2 Scope boundary

In scope:
- A persistent Go daemon (`lspd`) that runs per-user and manages a pool of LSP servers.
- An MCP server exposed at `http://127.0.0.1:<port>/mcp` over StreamableHTTP, impersonating the Droid VS Code extension.
- A Unix socket server for the Read-time PostToolUse hook.
- A thin Go binary (`lsp-read-hook`) that reads hook JSON on stdin, queries the daemon, and emits `hookSpecificOutput.additionalContext`.
- A launcher wrapper script that ensures the daemon is up and exports `FACTORY_VSCODE_MCP_PORT` before `exec droid`.
- A SessionStart hook as a fallback path for users who don't install the launcher wrapper.
- Configuration in YAML with per-user and per-project overrides.
- Structured logging and optional Prometheus metrics.

Out of scope for this project:
- Serena integration or interop. Serena remains an optional MCP plugin the user may install separately; `lspd` does not know about it.
- Droid codebase changes. `lspd` rides on Droid's existing extension points (`FACTORY_VSCODE_MCP_PORT`, the hook system, `fetchDiagnostics`/`compareDiagnostics`/`formatDiagnosticsForSystemReminder` pipeline) without modification.
- VS Code / JetBrains IDE extensions. We are *replacing* the need for those in terminal-based Droid sessions, not augmenting them.

### 1.3 Success criteria

- Running `droid` via the launcher wrapper causes `lspd` to start automatically if not already running.
- After any `Edit` on a TypeScript, Python, Go, Rust, or C/C++ file in a project, Droid's agent turn immediately following the edit contains a `<system-reminder>` block listing every new error or warning the relevant LSP server reports for that file. First-edit latency ≤ 2.5 seconds; warm-edit latency ≤ 500 ms.
- After any `Read` of a source file, the agent turn immediately following the read contains a `<system-reminder>` block listing the current known errors/warnings for that file (if any), peeked from the daemon's store without triggering a reparse.
- The model can call `lspDefinition`, `lspReferences`, `lspHover`, `lspWorkspaceSymbol`, `lspDocumentSymbol`, `lspCodeActions`, `lspRename`, `lspFormat`, `lspCallHierarchy`, and `lspTypeHierarchy` as native IDE tools, with responses formatted for LLM consumption.
- Diagnostic dedup: the same `(file, line, column, code, message)` tuple is never surfaced twice within a single Droid session.
- Volume caps: no single turn contains more than 50 diagnostics total and no single file contributes more than 20.
- Daemon crash recovery: if a language server process dies, it is restarted automatically with exponential backoff, and all previously-open documents are re-registered via `didOpen` on restart.
- Daemon survives Droid session end and remains warm for the next session (configurable idle timeout, default 30 minutes).
- Daemon survives SIGHUP by reloading config without dropping LSP connections.

---

## 2. Background

### 2.1 How Claude Code delivers LSP diagnostics

Claude Code's minified `cli.js` contains ~60 mentions of "diagnostic" and a clear pipeline inferrable from the visible string literals:

```
LSP Diagnostics: Registering N diagnostic file(s) from <server> for async delivery
LSP Diagnostics: Delivering N file(s) with M diagnostic(s) from K server(s)
LSP Diagnostics: Deduplication removed X duplicate diagnostic(s)
LSP Diagnostics: Volume limiting removed X diagnostic(s) (max N/file, M total)
LSP Diagnostics: Found N pending diagnostic set(s)
```

The architecture these strings imply:

1. Claude Code holds one or more JSON-RPC connections to LSP servers (either directly in headless mode, or via the IDE extension in IDE mode).
2. Each server's `textDocument/publishDiagnostics` notifications arrive asynchronously and are registered into a per-server "pending" store keyed by document URI, tagged with a UUID for the registration batch.
3. At the next `Read`/`Edit`/`Write` tool boundary, Claude Code drains the pending registry, dedupes against a per-session "already delivered" set, applies volume caps (per file, per delivery), sorts by severity, formats as an attachment of type `"diagnostics"`, and inlines it on the tool response.
4. The model sees the diagnostics in the same turn as the tool result with no round-trip.

The key architectural property is that diagnostics ride *with* the tool result rather than being fetched on demand. This is what "instant LSP feedback" means in practice.

### 2.2 Droid's existing IDE integration

Droid already implements the same pipeline for VS Code, Cursor, Windsurf, and JetBrains users. The evidence is in `droid-source-code/src/`:

#### 2.2.1 The MCP transport

`src/services/VSCodeIdeClient.ts` line 108:

```ts
const port = portOverride ?? process.env.FACTORY_VSCODE_MCP_PORT;
// ...
const serverUrl = `http://localhost:${port}/mcp`;
this.transport = new StreamableHTTPClientTransport(new URL(serverUrl));
```

The "IDE client" is an MCP client over StreamableHTTP. The port is configured by the `FACTORY_VSCODE_MCP_PORT` environment variable. Whatever listens on that port is treated as the IDE. Droid does not care whether it's the real VS Code extension or something impersonating it, as long as the MCP protocol and tool contract are honored.

`src/services/IdeContextManager.ts` line 65:

```ts
if (process.env.FACTORY_VSCODE_MCP_PORT) {
  const port = parseInt(process.env.FACTORY_VSCODE_MCP_PORT, 10);
  // ...creates VSCodeIdeClient and connects...
}
```

The context manager checks the env var during startup and connects automatically. No user action required beyond setting the variable before `droid` launches.

#### 2.2.2 The diagnostic pipeline

`src/tools/executors/client/file-tools/diagnostics-utils.ts` provides three functions that together implement the same push-delivery pipeline as Claude Code:

- **`fetchDiagnostics(ideClient, filePath, maxRetries=0, delayMs=500)`** (line 38): Calls `ideClient.callTool('getIdeDiagnostics', { uri })`, parses the response, and returns `IdeDiagnostic[]`. Optional retry loop with delay for cases where the language server hasn't reparsed yet.

- **`compareDiagnostics(before, after)`** (line 111): Takes two snapshots and returns only diagnostics present in `after` but not in `before`. Filters to severity 0 (Error) and 1 (Warning) only. Comparison key is `(message, range.start.line, severity)`. This is the dedup layer: it prevents old errors from being re-surfaced.

- **`formatDiagnosticsForSystemReminder(newDiagnostics, filePath)`** (line 143): Formats the filtered diagnostics as a `<system-reminder>` block grouped by severity:

  ```
  <system-reminder>
  New errors detected after editing foo.ts:
  Errors:
    - Line 12: Cannot find name 'fetchUsers' (pyright)
    - Line 18: Expected 2 arguments, but got 1 (ts)

  Warnings:
    - Line 25: Unused variable 'tmp' (ts)
  </system-reminder>
  ```

#### 2.2.3 Integration call sites

Three tool executors call this pipeline today:

| File | Lines | Pattern |
|---|---|---|
| `src/tools/executors/client/edit-cli.ts` | 104–161 | `diagnosticsBefore` → edit → `diagnosticsAfter` (1 retry, 500ms) → `compareDiagnostics` → `formatDiagnosticsForSystemReminder` → attach to result |
| `src/tools/executors/client/create-cli.ts` | 191–202 | `diagnosticsAfter` only (nothing before a create) → format → attach |
| `src/tools/executors/client/apply-patch-cli.ts` | 248, 343–358 | Same as edit-cli but for patches |

The `<system-reminder>` string is appended to the tool's response value, so it flows to the agent as part of the same tool result.

#### 2.2.4 The Read gap

`src/tools/executors/client/read-cli.ts` does *not* call `fetchDiagnostics`. Grep confirms zero references to diagnostics in that file. This is the "Read gap": Droid currently delivers diagnostics after Edit/Create/Write but not after Read. We close this gap via a PostToolUse hook, not by modifying Droid.

#### 2.2.5 MCP tools Droid calls on the IDE

`src/services/VSCodeIdeClient.ts` has four `callTool` call sites:

```ts
await this.callTool('getIdeDiagnostics', { uri });         // line 52 (via diagnostics-utils)
await this.callTool('openDiff', { filePath, newContent }); // line 404
await this.callTool('closeDiff', { filePath });            // line 412
await this.callTool('openFile', { filePath, waitForSave }); // line 420
```

Only `getIdeDiagnostics` needs real behavior. The other three can be no-op stubs that return success — Droid calls them but tolerates them being cosmetic when there's no visible IDE.

#### 2.2.6 The expected Diagnostic shape

From the field accesses in `fetchDiagnostics`, `compareDiagnostics`, and `formatDiagnosticsForSystemReminder`, the `IdeDiagnostic` shape Droid expects is:

```ts
interface IdeDiagnostic {
  severity: number;              // 0 = Error, 1 = Warning, 2 = Info, 3 = Hint
  message: string;
  source?: string;               // "ts", "pyright", "gopls", etc.
  range: {
    start: { line: number; character: number };  // 0-indexed
    end:   { line: number; character: number };
  };
  code?: string | number;
}
```

This is the exact LSP `Diagnostic` shape. Zero translation needed between what the LSP server publishes and what Droid expects. The MCP `getIdeDiagnostics` response wraps it as:

```json
{ "diagnostics": [ /* IdeDiagnostic[] */ ] }
```

### 2.3 Droid's hook system

The hook system is the path we use to fill the Read gap. Source references:

#### 2.3.1 Spawn model

`src/services/HookService.ts` line 274:

```ts
const child = spawn(shell, shellArgs, {
  env,
  timeout: timeout * 1000,
});
```

No `detached: true`. No stdio override. stdin/stdout/stderr are pipes attached to the parent. Implication: a hook command that starts a child process and returns control (without fully detaching stdin/stdout/stderr) will keep the hook promise alive until the child exits, because Node keeps reading from the pipes. Any hook that launches a long-running daemon must either use `nohup … > /path/log 2>&1 < /dev/null & disown` or have the child self-daemonize (double-fork + `setsid` + close fds).

#### 2.3.2 Environment injection

`src/services/HookService.ts` lines 242–254:

```ts
for (const [key, value] of Object.entries(compatInput)) {
  if (validEnvVarPattern.test(key)) {
    if (typeof value === 'string') {
      inputEnvVars[key] = value;
    } else if (typeof value === 'boolean' || typeof value === 'number') {
      inputEnvVars[key] = String(value);
    }
  }
}
```

Every string/number/bool field in the hook input JSON is exposed as an environment variable to the hook command, provided the name matches `^[a-zA-Z_][a-zA-Z0-9_]*$`. So `$session_id`, `$cwd`, `$hook_event_name`, `$transcript_path`, `$permission_mode`, `$source`, `$reason`, `$tool_name` are all available as shell variables without parsing stdin JSON in the common case.

#### 2.3.3 The `CLAUDE_ENV_FILE` escape hatch

`src/services/SessionService.ts` lines 3812–3859:

```ts
CLAUDE_ENV_FILE: envFilePath,
// ...
if (fs.existsSync(envFilePath)) {
  const envContent = fs.readFileSync(envFilePath, 'utf-8');
  // parse "export KEY=value" lines and apply to process.env
}
```

SessionStart hooks can write `export KEY=value` lines to `$CLAUDE_ENV_FILE` (a path Droid passes in the input), and Droid reads the file after the hook completes and applies the variables to its own `process.env`. These variables persist for the lifetime of the Droid process and are inherited by every subsequent hook invocation and tool executor. This is how a SessionStart hook can inject `FACTORY_VSCODE_MCP_PORT` into Droid's own environment.

**Caveat**: `IdeContextManager` reads `FACTORY_VSCODE_MCP_PORT` at construction time, which happens very early in Droid startup — potentially *before* SessionStart hooks run. If that's the case, the `CLAUDE_ENV_FILE` path won't work to inject the port for the IDE client connection, and the launcher-wrapper path is required. **This is verification item #1 in §18.**

#### 2.3.4 PostToolUse synchronous injection

`src/core/ToolExecutor.ts` lines 1385–1425:

```ts
const hookResults = await executeHooksWithDisplay(
  HookEventName.PostToolUse,
  {
    session_id: hookSessionId,
    transcript_path: transcriptPath,
    cwd: process.cwd(),
    permission_mode: getPermissionModeString(currentMode),
    hook_event_name: HookEventName.PostToolUse,
    tool_name: toolUse.name,
    tool_input: toolUse.input,
    tool_response: result,
  },
  toolUse.name,
  { updateAction: this.updateAction, sessionId: hookSessionId }
);

// ...

// 2. Add additionalContext to conversation
const contextResult = hookResults.find(
  (r) => r.hookSpecificOutput?.additionalContext
);
if (
  contextResult?.hookSpecificOutput?.additionalContext &&
  this.updateAction
) {
  this.updateAction({
    type: 'ADD_MESSAGE',
    role: MessageRole.System,
    content: contextResult.hookSpecificOutput.additionalContext,
  });
}
```

Two things matter:

1. The `await` on line 1385 makes PostToolUse hook execution **synchronous**. Droid does not return the tool result to the agent loop until every matched hook has completed.
2. The `ADD_MESSAGE` dispatch on line 1420 injects `additionalContext` as a System-role message into the conversation **before** the executor returns. The next LLM call sees both the tool result and the system message in the same request.

This is the same delivery guarantee as the native IDE path: the diagnostics are in front of the model before the model gets to make its next decision.

#### 2.3.5 SessionStart / SessionEnd

`src/services/SessionService.ts`:

- **SessionStart** (line 3801): Droid `await`s the hook before proceeding with session initialization. Hook input contains `session_id`, `transcript_path`, `cwd`, `permission_mode`, `hook_event_name`, `source` (`startup|resume|clear|compact`), `previous_session_id`, `calling_session_id`, and the special `CLAUDE_ENV_FILE` path. `additionalContext` from the hook gets stashed in `pendingSessionStartContext` and injected on the first user message of the session.
- **SessionEnd** (line 3681): Fires via `shutdownCoordinator` at shutdown priority `SHUTDOWN_HOOK_PRIORITY.SessionEnd`. Guarded by `sessionEndHooksExecuted` (one-shot per process lifetime). Handles graceful exit, `/quit`, `/clear`, logout, and SIGINT via the shutdown coordinator. **Not guaranteed on SIGKILL or hard crashes** — daemon must have its own idle-timeout cleanup as a safety net.

#### 2.3.6 Hook snapshot at startup

From the hooks reference documentation (corroborated by the presence of a snapshot mechanism referenced in `useHooksManager.ts`): Droid captures a snapshot of the hook configuration at session startup and uses that snapshot for the entire session. Editing `settings.json` mid-session has no effect until the next session or a `/hooks` review. This means we can't hot-reload hook definitions, but we *can* hot-reload the daemon config (which is independent of the hook config).

### 2.4 LSP fundamentals we rely on

For readers unfamiliar with LSP, this section documents the specific LSP methods and notifications `lspd` depends on. The canonical spec is the [Microsoft Language Server Protocol Specification 3.17](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/).

#### 2.4.1 Initialization

- `initialize` request: client → server. Establishes workspace folders, client capabilities (we claim `publishDiagnostics`, `definition`, `references`, `hover`, `documentSymbol`, `workspaceSymbol`, `codeAction`, `rename`, `formatting`, `callHierarchy`, `typeHierarchy`), server capabilities negotiation.
- `initialized` notification: client → server. Sent after `initialize` completes.
- `workspace/didChangeConfiguration` notification: client → server. Pushes per-language settings (pyright typeCheckingMode, gopls staticcheck, etc.).

#### 2.4.2 Document lifecycle

- `textDocument/didOpen` notification: client → server. Registers a document with the server. Contains full text, URI, language ID, version number.
- `textDocument/didChange` notification: client → server. Tells the server the document content has changed. We use full-text sync (simpler than incremental range edits), with an incremented version number. Every time Droid or a file-watcher detects a change, we send `didChange` with the new contents.
- `textDocument/didClose` notification: client → server. Removes a document. Sent when a document hasn't been touched in 15 minutes (configurable) to free server memory.
- `textDocument/didSave` notification: client → server. Some servers (gopls in particular) use this to trigger re-analysis.

#### 2.4.3 Diagnostic push

- `textDocument/publishDiagnostics` notification: server → client. The async push we care about most. Arrives at unpredictable times after `didChange`. Payload is `{ uri, version?, diagnostics: Diagnostic[] }`. Latest-wins semantics: a new publish for the same URI replaces the previous publish entirely.
- `textDocument/diagnostic` request (LSP 3.17+ "pull diagnostics"): client → server. Optional. Some servers support pulling diagnostics on demand instead of relying on push. We prefer push for diagnostic push delivery but can fall back to pull for on-demand queries.

#### 2.4.4 Navigation

- `textDocument/definition` request: returns `Location | LocationLink`.
- `textDocument/references` request: returns `Location[]`.
- `textDocument/hover` request: returns `Hover` with `contents: MarkupContent | MarkedString[]`.
- `textDocument/documentSymbol` request: returns `DocumentSymbol[]` (hierarchical) or `SymbolInformation[]` (flat).
- `workspace/symbol` request: returns `SymbolInformation[]` for a fuzzy query.
- `textDocument/codeAction` request: returns `CodeAction[]` with `edit: WorkspaceEdit` or `command`.
- `textDocument/rename` request: returns `WorkspaceEdit`.
- `textDocument/formatting` and `textDocument/rangeFormatting` requests: return `TextEdit[]`.
- `textDocument/prepareCallHierarchy` + `callHierarchy/incomingCalls` + `callHierarchy/outgoingCalls` requests: returns `CallHierarchyItem[]` and `CallHierarchy*Call[]`.
- `textDocument/prepareTypeHierarchy` + `typeHierarchy/supertypes` + `typeHierarchy/subtypes` requests: returns `TypeHierarchyItem[]`.

#### 2.4.5 Server-specific quirks we already know about

- **pyright** requires the `initialized` notification to be sent *before* any `didOpen`, otherwise first-file diagnostics race and never arrive.
- **typescript-language-server** requires an explicit `workspace/didChangeConfiguration` with at least an empty object, otherwise it never emits diagnostics.
- **gopls** requires workspace folders set at `initialize` time, otherwise it runs in single-file mode with degraded analysis. Also prefers `didSave` notifications for certain analyses.
- **rust-analyzer** has heavy indexing on startup; warmup is expensive. Diagnostics don't flow until `rust-analyzer/serverStatus` reports `ready`.
- **clangd** needs `compile_commands.json` or `.clangd` to do anything useful. In its absence, diagnostics are limited to syntactic errors.

---

## 3. Architecture

### 3.1 Overview diagram

```
                   ┌──────────────────────────────────────────┐
                   │                 Droid                     │
                   │                                           │
                   │  Read/Edit/Create/Write tool executors    │
                   │                  │                        │
                   │                  │ fetchDiagnostics       │
                   │                  │                        │
                   │  ┌───────────────▼────────────────┐       │
                   │  │       VSCodeIdeClient          │       │
                   │  │  MCP client over StreamableHTTP│       │
                   │  └───────────────┬────────────────┘       │
                   │                  │                        │
                   │   PostToolUse    │                        │
                   │   hook (Read)    │                        │
                   │        │         │                        │
                   └────────┼─────────┼────────────────────────┘
                            │         │
                 (Unix socket)   (HTTP MCP)
                            │         │
                            ▼         ▼
          ┌────────────────────────────────────────────┐
          │                   lspd                     │
          │                                             │
          │   ┌──────────┐      ┌───────────────┐      │
          │   │  Socket  │      │ MCP server     │     │
          │   │  server  │      │ (mcp-go,       │     │
          │   │  (drain, │      │  StreamableHTTP│     │
          │   │  peek,   │      │  transport)    │     │
          │   │  etc.)   │      └───────┬────────┘     │
          │   └────┬─────┘              │              │
          │        │                    │              │
          │        └──────┬─────────────┘              │
          │               │                            │
          │   ┌───────────▼──────────────┐             │
          │   │   Policy / Dedup /       │             │
          │   │   Volume-cap engine      │             │
          │   └───────────┬──────────────┘             │
          │               │                            │
          │   ┌───────────▼──────────────┐             │
          │   │   DiagnosticStore        │             │
          │   │   (versioned, per-URI)   │             │
          │   └───────────┬──────────────┘             │
          │               │                            │
          │               │  publishDiagnostics        │
          │               │                            │
          │   ┌───────────▼──────────────┐             │
          │   │   LSP ClientManager pool │             │
          │   │  ┌──────┐ ┌──────┐ ┌────┐│             │
          │   │  │  ts  │ │  py  │ │ go ││             │
          │   │  └──┬───┘ └──┬───┘ └──┬─┘│             │
          │   └─────┼────────┼────────┼──┘             │
          │         │        │        │                │
          │   tsserver   pyright    gopls              │
          │   (stdio     (stdio     (stdio             │
          │    subproc)   subproc)   subproc)          │
          │                                            │
          │   ┌──────────────────────────┐             │
          │   │  fsnotify file watcher   │             │
          │   │  (background reparse on  │             │
          │   │   out-of-band edits)     │             │
          │   └──────────────────────────┘             │
          │                                            │
          │   ┌──────────────────────────┐             │
          │   │  Supervisor (crash       │             │
          │   │  recovery, backoff)      │             │
          │   └──────────────────────────┘             │
          │                                            │
          │   ┌──────────────────────────┐             │
          │   │  Metrics HTTP endpoint   │             │
          │   │  (Prometheus, opt-in)    │             │
          │   └──────────────────────────┘             │
          └────────────────────────────────────────────┘
```

### 3.2 Component map

- **`cmd/lspd`** — Binary entrypoint with CLI subcommands (start, stop, status, reload, logs, diag, fix, ping).
- **`cmd/lsp-read-hook`** — Tiny thin-client binary invoked by Droid's PostToolUse Read hook. Reads hook JSON, queries the daemon over Unix socket, emits `hookSpecificOutput.additionalContext`.
- **`internal/daemon`** — Lifecycle management: self-daemonize (double-fork + setsid), signal handling (SIGHUP reload, SIGTERM graceful shutdown), idle timeout, pidfile, portfile, single-instance lock.
- **`internal/lsp`** — LSP orchestration. Sub-packages:
  - `internal/lsp/client` — Per-server `ClientManager`: stdio subprocess, JSON-RPC framing, document lifecycle, notification routing.
  - `internal/lsp/router` — Extension → language routing and project root detection.
  - `internal/lsp/supervisor` — Restart logic with exponential backoff, health tracking.
  - `internal/lsp/store` — `DiagnosticStore` with versioned latest-wins semantics and wait channels.
  - `internal/lsp/docs` — Open document tracker with TTL-based eviction.
- **`internal/mcp`** — MCP server. Sub-packages:
  - `internal/mcp/transport` — StreamableHTTP transport integration via `mcp-go`.
  - `internal/mcp/tools/compat` — Tier 1 IDE-compatibility tools (`getIdeDiagnostics`, `openDiff`, `closeDiff`, `openFile`).
  - `internal/mcp/tools/nav` — Tier 2 semantic navigation tools.
  - `internal/mcp/descriptions` — Tool description strings (extracted so they can be tuned independently; descriptions are load-bearing for model behavior).
- **`internal/socket`** — Unix socket server for `drain`, `peek`, `forget`, `status`, `ping` operations used by the hook client.
- **`internal/policy`** — Pure functions: dedup, volume caps, severity filter, source allow/deny, code-action attachment.
- **`internal/config`** — YAML loader with schema validation, per-project overrides, hot-reload on SIGHUP.
- **`internal/watcher`** — fsnotify file watcher. On out-of-band file changes, pushes `didChange` to the relevant LSP server so the store stays fresh.
- **`internal/metrics`** — Optional Prometheus endpoint for `lspd_*` metrics.
- **`internal/log`** — Structured JSON logger with rotation.
- **`internal/format`** — Formatting helpers: diagnostics → system-reminder string, LSP types → LLM-friendly JSON.

### 3.3 Data flow (high level)

**Edit flow**: Droid's `edit-cli.ts` calls `fetchDiagnostics` before the edit → `VSCodeIdeClient.callTool('getIdeDiagnostics', {uri})` → HTTP POST to `lspd` MCP endpoint → `internal/mcp/tools/compat.GetIdeDiagnostics` handler → router finds the language manager for this file extension → if document not yet open, send `didOpen`; otherwise rely on last-known store entry → return current store snapshot for this URI. Edit happens. `fetchDiagnostics` called again with 1 retry, 500ms delay → `lspd` receives the request → sends `didChange` with updated content → blocks in `DiagnosticStore.Wait(uri, version≥N, timeout=1200ms)` for publishDiagnostics to arrive → applies policy (dedup against session, volume caps, severity filter) → returns. Droid runs `compareDiagnostics(before, after)` → formats `<system-reminder>` → attaches to tool result. Agent sees diagnostics in the same turn as the edit.

**Read flow**: Droid's `read-cli.ts` returns file contents to the agent. PostToolUse hook fires synchronously → `lsp-read-hook` binary runs → reads hook JSON from stdin → connects to `/tmp/droid-lspd.sock` → sends `{"op": "peek", "path": "..."}` → `lspd` returns current diagnostic store state for that URI *without* triggering a reparse (this is a pure read) → applies policy → returns → hook binary formats as system-reminder, emits as `hookSpecificOutput.additionalContext` JSON, exits 0 → Droid `ToolExecutor.ts` collects the hook output → dispatches `ADD_MESSAGE` with System role → agent's next turn sees both the file contents and the diagnostics.

**Navigation flow** (e.g., model asks for all references to a function): Agent calls the `lspReferences` tool on the IDE endpoint → HTTP POST to lspd MCP endpoint → `internal/mcp/tools/nav.LspReferences` handler → router finds the right language manager → sends `textDocument/references` request → waits on response → returns LLM-friendly JSON to Droid → Droid forwards to the model.

---

## 4. The daemon: `lspd`

### 4.1 Process model

- **One daemon per user.** Pidfile at `~/.factory/run/lspd.pid`. Portfile at `~/.factory/run/lspd.port`. Unix socket at `~/.factory/run/lspd.sock`. All three directories created with mode 0700 on first startup.
- **Single-instance lock** via `flock(2)` on the pidfile. `lspd start` checks the lock; if held, it reads the existing port from `lspd.port` and exits with status 0 (idempotent start).
- **Global user scope** (not per-project). The same daemon serves every project root the user opens. Multi-root LSP servers use `workspace/didChangeWorkspaceFolders` to add new roots as the user switches projects. This amortizes the LSP server cold-start cost across sessions.

### 4.2 CLI subcommands

| Subcommand | Behavior |
|---|---|
| `lspd start [--foreground] [--config PATH]` | Starts the daemon. Without `--foreground`, self-daemonizes via double-fork. Prints `FACTORY_VSCODE_MCP_PORT=NNNN` to stdout (captured by the launcher wrapper) before detaching. Idempotent: if already running, prints existing port and exits 0. |
| `lspd stop [--force]` | Sends SIGTERM for graceful shutdown (LSP `shutdown`+`exit`, flush logs). With `--force`, sends SIGKILL after 5s timeout. |
| `lspd reload` | Sends SIGHUP. Daemon reloads config: new language definitions available on next file touch, already-running servers stay up, policy changes apply immediately. |
| `lspd status [--json]` | Queries the socket `status` op. Human-readable table by default; JSON with `--json`. Shows per-language pid/uptime/document count/restart count/last publish timestamp, sessions list, MCP port, policy summary. |
| `lspd logs [--follow] [--since DURATION]` | Tails the structured log file. Equivalent to `tail -f ~/.factory/logs/lspd.log` but with optional structured filtering. |
| `lspd diag PATH [--json]` | Ad-hoc diagnostic query for a file. Prints current store contents. Useful for debugging and for `droid exec` scripts. |
| `lspd fix PATH:LINE [--json]` | Ad-hoc code-action query. Prints available LSP code actions at a position. Debugging aid. |
| `lspd ping` | Checks whether the daemon is alive. Returns 0 if responsive, non-zero otherwise. Used by the launcher wrapper. |
| `lspd version` | Prints binary version. |

### 4.3 Self-daemonization

Standard Unix double-fork dance:

1. Parent process (the `lspd start` command) calls `fork()`. Parent exits immediately, returning control to the caller shell.
2. First child calls `setsid()` to become its own session leader, detaching from the controlling terminal.
3. First child `fork()`s a grandchild. First child exits so the grandchild cannot re-acquire a terminal.
4. Grandchild calls `chdir("/")` to avoid holding any working directory.
5. Grandchild `umask(0o077)` for tight default permissions.
6. Grandchild prints the chosen MCP port to stdout (before closing it) so the launcher wrapper can read it.
7. Grandchild closes stdin, stdout, stderr. Redirects them to `/dev/null` (or, if log_file is configured, to the log file for stdout/stderr).
8. Grandchild enters the main loop.

Caveat: Go's runtime doesn't make `fork()` trivial because of the runtime threads. The clean pattern is to re-exec with a sentinel env var: `lspd start` writes the port to a temp file, then execs `lspd daemonize --port N` which on startup detects the sentinel and skips the fork step. This is the approach used by several Go daemons including caddy and influxdb.

### 4.4 Lifecycle

- **Start**: `lspd start` → check lock → if not held, acquire → self-daemonize → load config → start metrics endpoint (if enabled) → start socket listener → start MCP server on free port → write port to portfile → write pid to pidfile → enter main loop.
- **Main loop**: Select on socket accepts, MCP requests, SIGHUP, SIGTERM, idle timer tick, per-language LSP supervisor events.
- **Idle timeout**: Reset on every MCP request, every socket request, every `publishDiagnostics` from any LSP server. When idle for `idle_timeout` (default 30min), initiate graceful shutdown. The rationale is that the daemon should outlive individual Droid sessions (so the next session doesn't pay a cold-start cost) but eventually release resources when nothing is happening.
- **SIGHUP**: Reload config from disk. Diff against current config: changed language definitions mark those managers for restart on next touch; unchanged managers keep running; removed languages get graceful shutdown; added languages are available lazily on next file touch.
- **SIGTERM**: Graceful shutdown. For each LSP client: send `shutdown` request, wait up to 2s for response, send `exit` notification, kill process if still alive after 1s. Close MCP server (drain in-flight requests with 5s timeout). Close socket listener. Flush logs. Remove pidfile, portfile, socket path. Release lock. Exit 0.
- **Panic**: Recover in every goroutine. Log the stack trace. Mark the affected component as degraded. Never crash the whole daemon because one LSP server misbehaved.

### 4.5 Crash recovery / supervision

See §5.3 for the full supervisor. Summary: each LSP child process is watched by a dedicated goroutine. On unexpected exit (non-zero code, signal other than clean shutdown), the supervisor restarts with exponential backoff (1s, 2s, 4s, 8s, capped at 8s) up to `max_restarts` within a 10-minute rolling window (default 5). After the limit, the language enters "degraded" state: subsequent requests for that language return empty results with a log warning, not an error, so Droid edits don't fail because a language server is broken. Degraded state clears on `lspd reload` or on the next successful health check (we attempt a probe every 60s).

On restart, the supervisor re-runs `initialize`, re-sends `workspace/didChangeConfiguration`, re-sends `didOpen` for every document the client manager knew about. This makes restarts transparent to callers.

---

## 5. LSP orchestration layer

### 5.1 Client manager (`internal/lsp/client`)

One `ClientManager` per configured language. Responsibilities:

- **Spawn the subprocess.** Stdin/stdout pipes, stderr captured to the structured log with `language=<name>` tag. Environment inherits parent plus any `env:` from config.
- **Framing.** LSP uses `Content-Length: N\r\n\r\n<JSON>` framing. The `go.lsp.dev/jsonrpc2` library handles this; we wrap it in a `Conn`.
- **Initialize handshake.** Build `InitializeParams` with workspace folders (resolved from the configured `root_markers` for this language starting from the first touched file), client capabilities (declaring support for all 10 navigation methods plus `publishDiagnostics`), and `initializationOptions` from config. Send `initialize` request. On response, send `initialized` notification. Send `workspace/didChangeConfiguration` if `settings:` is configured.
- **Notification routing.** Start a reader goroutine that receives all incoming JSON-RPC messages and routes:
  - `textDocument/publishDiagnostics` → `DiagnosticStore.Publish(uri, version, diagnostics)`
  - `window/logMessage` → structured log with `source=lsp`, `language=<name>`
  - `window/showMessage` → same, at higher level
  - `$/progress` → progress tracker used by `lspd status`
  - unknown notifications → debug log and ignore
- **Request wrappers.** Typed Go methods for each LSP method we support: `Definition(ctx, params)`, `References(ctx, params)`, `Hover(ctx, params)`, `DocumentSymbol(ctx, params)`, `WorkspaceSymbol(ctx, params)`, `CodeAction(ctx, params)`, `Rename(ctx, params)`, `Formatting(ctx, params)`, `PrepareCallHierarchy(ctx, params)`, `IncomingCalls(ctx, params)`, `OutgoingCalls(ctx, params)`, `PrepareTypeHierarchy(ctx, params)`, `Supertypes(ctx, params)`, `Subtypes(ctx, params)`. Each is a thin wrapper around `jsonrpc2.Call`.
- **Document lifecycle.** `EnsureOpen(uri, content)` sends `didOpen` the first time and `didChange` on subsequent calls, tracking version. `Close(uri)` sends `didClose`. `Touch(uri)` updates the `lastAccessed` timestamp. A background sweeper closes documents not touched in 15 minutes.
- **Warmup.** If `warmup: true` in config, after `initialized` the manager issues a `workspace/symbol` request with an empty query to force the server to build its index. This turns the first real `lspDefinition` call from ~10s on a cold pyright into ~100ms on a warm pyright.
- **Shutdown.** `Shutdown(ctx)` sends `shutdown` request, waits, then `exit` notification, then kills the process if it hasn't exited within 1s.

### 5.2 Router (`internal/lsp/router`)

Extension-to-language mapping, with per-directory override capability.

```go
type Router struct {
    byExt         map[string]*Language       // ".ts" → ts language
    managers      map[string]*client.ClientManager // keyed by language name
    managersMu    sync.RWMutex
    cfg           *config.Config
}

func (r *Router) Resolve(ctx context.Context, path string) (*client.ClientManager, error) {
    ext := filepath.Ext(path)
    lang, ok := r.byExt[ext]
    if !ok {
        return nil, ErrUnsupportedLanguage
    }

    r.managersMu.RLock()
    mgr, ok := r.managers[lang.Name]
    r.managersMu.RUnlock()
    if ok {
        return mgr, nil
    }

    // Lazy spawn
    r.managersMu.Lock()
    defer r.managersMu.Unlock()
    if mgr, ok := r.managers[lang.Name]; ok {
        return mgr, nil
    }
    root := r.findProjectRoot(path, lang.RootMarkers)
    mgr, err := client.NewClientManager(ctx, lang, root)
    if err != nil {
        return nil, err
    }
    r.managers[lang.Name] = mgr
    return mgr, nil
}

func (r *Router) findProjectRoot(path string, markers []string) string {
    dir := filepath.Dir(path)
    for {
        for _, m := range markers {
            if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
                return dir
            }
        }
        parent := filepath.Dir(dir)
        if parent == dir {
            return filepath.Dir(path) // no marker found, fall back to file's dir
        }
        dir = parent
    }
}
```

### 5.3 Supervisor (`internal/lsp/supervisor`)

Wraps each `ClientManager` with restart logic.

```go
type Supervisor struct {
    lang          *config.Language
    mgr           *client.ClientManager
    restarts      []time.Time  // sliding window
    maxRestarts   int
    restartWindow time.Duration
    state         State // healthy | restarting | degraded
    lastErr       error
}

func (s *Supervisor) run(ctx context.Context) {
    for {
        err := s.mgr.Wait() // blocks until process exits
        if ctx.Err() != nil {
            return // graceful shutdown
        }
        s.lastErr = err
        s.restarts = append(s.restarts, time.Now())
        s.pruneOldRestarts()
        if len(s.restarts) >= s.maxRestarts {
            s.state = StateDegraded
            log.Warnf("language %s entered degraded state after %d restarts", s.lang.Name, len(s.restarts))
            return
        }
        backoff := s.backoffDuration(len(s.restarts))
        time.Sleep(backoff)
        if err := s.restart(ctx); err != nil {
            log.Errorf("failed to restart language %s: %v", s.lang.Name, err)
            continue
        }
    }
}

func (s *Supervisor) restart(ctx context.Context) error {
    newMgr, err := client.NewClientManager(ctx, s.lang, s.mgr.Root())
    if err != nil {
        return err
    }
    // Re-open all previously tracked documents
    for _, doc := range s.mgr.TrackedDocs() {
        newMgr.EnsureOpen(doc.URI, doc.Content)
    }
    s.mgr = newMgr
    s.state = StateHealthy
    return nil
}
```

### 5.4 Diagnostic store (`internal/lsp/store`)

Versioned latest-wins store with wait channels.

```go
type DiagnosticStore struct {
    mu      sync.RWMutex
    entries map[protocol.DocumentURI]*entry
    waiters map[protocol.DocumentURI][]*waiter
}

type entry struct {
    version     int32
    diagnostics []protocol.Diagnostic
    updatedAt   time.Time
    source      string // language name for metrics
}

type waiter struct {
    minVersion int32
    ch         chan struct{}
    deadline   time.Time
}

func (s *DiagnosticStore) Publish(uri protocol.DocumentURI, version int32, diags []protocol.Diagnostic, source string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.entries[uri] = &entry{
        version:     version,
        diagnostics: diags,
        updatedAt:   time.Now(),
        source:      source,
    }
    // Signal any waiters
    for i, w := range s.waiters[uri] {
        if version >= w.minVersion {
            close(w.ch)
            s.waiters[uri][i] = nil
        }
    }
    s.waiters[uri] = compact(s.waiters[uri])
}

func (s *DiagnosticStore) Peek(uri protocol.DocumentURI) ([]protocol.Diagnostic, int32) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    e, ok := s.entries[uri]
    if !ok {
        return nil, 0
    }
    return e.diagnostics, e.version
}

func (s *DiagnosticStore) Wait(ctx context.Context, uri protocol.DocumentURI, minVersion int32, timeout time.Duration) ([]protocol.Diagnostic, int32, error) {
    s.mu.Lock()
    if e, ok := s.entries[uri]; ok && e.version >= minVersion {
        s.mu.Unlock()
        return e.diagnostics, e.version, nil
    }
    w := &waiter{
        minVersion: minVersion,
        ch:         make(chan struct{}),
        deadline:   time.Now().Add(timeout),
    }
    s.waiters[uri] = append(s.waiters[uri], w)
    s.mu.Unlock()
    select {
    case <-w.ch:
        diagnostics, version := s.Peek(uri)
        return diagnostics, version, nil
    case <-time.After(timeout):
        diagnostics, version := s.Peek(uri)
        return diagnostics, version, context.DeadlineExceeded
    case <-ctx.Done():
        return nil, 0, ctx.Err()
    }
}
```

Note the semantics: `Wait` returns whatever the store has at deadline, even if it's stale, so the caller always has *something* to return to Droid. The error is informational.

### 5.5 Open document tracker

Tracked per-`ClientManager` as a `sync.Map[URI]*openDoc` with `content`, `version`, `lastAccessed`. A background goroutine sweeps every 60s and closes documents inactive for more than 15 minutes. On `EnsureOpen`, version is incremented and the change is sent to the server via `didChange` (or `didOpen` on first touch). On config reload, the tracker is preserved so we don't lose open state.

---

## 6. MCP server layer

### 6.1 Transport and registration

Uses `github.com/mark3labs/mcp-go` with its StreamableHTTP server transport.

```go
import (
    "github.com/mark3labs/mcp-go/server"
    "github.com/mark3labs/mcp-go/mcp"
)

func NewMCPServer(deps *Deps) *server.MCPServer {
    s := server.NewMCPServer(
        "lspd",
        Version,
        server.WithToolCapabilities(true),
        server.WithLogging(),
    )

    // Tier 1: Droid IDE compatibility
    compat.Register(s, deps)

    // Tier 2: Semantic navigation
    nav.Register(s, deps)

    return s
}

func Serve(ctx context.Context, s *server.MCPServer, addr string) error {
    httpServer := server.NewStreamableHTTPServer(s)
    return httpServer.Start(addr)
}
```

The MCP server binds to `127.0.0.1:0` (any free port), captures the chosen port, writes it to `~/.factory/run/lspd.port`, and prints it to stdout for the launcher wrapper to capture.

### 6.2 Tier 1: IDE compatibility tools

These four tools are what Droid's `VSCodeIdeClient` actually calls. Only `getIdeDiagnostics` has real behavior; the others are stubs that return success.

#### 6.2.1 `getIdeDiagnostics`

```go
type GetIdeDiagnosticsParams struct {
    URI string `json:"uri"`
}

type GetIdeDiagnosticsResponse struct {
    Diagnostics []IdeDiagnostic `json:"diagnostics"`
}

type IdeDiagnostic struct {
    Severity int             `json:"severity"` // 0=Error, 1=Warning, 2=Info, 3=Hint
    Message  string          `json:"message"`
    Source   string          `json:"source,omitempty"`
    Range    Range           `json:"range"`
    Code     json.RawMessage `json:"code,omitempty"` // string | number
}

type Range struct {
    Start Position `json:"start"`
    End   Position `json:"end"`
}

type Position struct {
    Line      int `json:"line"`      // 0-indexed
    Character int `json:"character"` // 0-indexed
}

func handleGetIdeDiagnostics(deps *Deps) server.ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        var p GetIdeDiagnosticsParams
        if err := req.BindArguments(&p); err != nil {
            return nil, err
        }
        path, err := uriToPath(p.URI)
        if err != nil {
            return emptyDiagnostics(), nil
        }
        mgr, err := deps.Router.Resolve(ctx, path)
        if err != nil {
            // Unsupported language — return empty, not error, so Droid edits don't fail
            return emptyDiagnostics(), nil
        }

        // Read current file contents and push didChange if we've been notified of a change
        content, err := os.ReadFile(path)
        if err == nil {
            mgr.EnsureOpen(protocol.DocumentURI(p.URI), string(content))
        }

        // Wait briefly for publish to land
        diags, _, _ := deps.Store.Wait(ctx, protocol.DocumentURI(p.URI), 0, 1200*time.Millisecond)

        // Apply policy (dedup, volume cap, severity filter)
        sessionID := mcp.SessionIDFromContext(ctx)
        filtered := deps.Policy.Apply(sessionID, p.URI, diags)

        return mcp.NewToolResultJSON(GetIdeDiagnosticsResponse{
            Diagnostics: toIdeDiagnostics(filtered),
        }), nil
    }
}
```

Error handling: never return an error from this tool. Empty diagnostics or partial results are preferable to failing the tool call, because Droid's edit-cli has no fallback — an error would break the edit flow. On any internal failure, log, return `{"diagnostics": []}`, continue.

#### 6.2.2 `openDiff`, `closeDiff`, `openFile`

```go
func handleStub(name string) server.ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        log.Debugf("stub call: %s with args %s", name, req.Params.Arguments)
        return mcp.NewToolResultText("ok"), nil
    }
}
```

These stubs return `"ok"` and log at debug level. Droid doesn't check the response content; it only checks that the tool call didn't error. The stubs intentionally don't try to replicate any IDE UI behavior.

### 6.3 Tier 2: Semantic navigation tools

All tier-2 tools share a common pattern: parse params, resolve via router, issue the LSP request, transform the response to LLM-friendly JSON, return.

#### 6.3.1 `lspDefinition`

```go
type LspDefinitionParams struct {
    Path      string `json:"path"`
    Line      int    `json:"line"`      // 1-indexed (LLM-friendly)
    Character int    `json:"character"` // 1-indexed
}

type LspDefinitionResponse struct {
    Definitions []Location `json:"definitions"`
}

type Location struct {
    Path  string `json:"path"`
    Line  int    `json:"line"`      // 1-indexed
    Column int   `json:"column"`    // 1-indexed
    EndLine   int `json:"end_line,omitempty"`
    EndColumn int `json:"end_column,omitempty"`
    Preview   string `json:"preview,omitempty"` // source line for context
}

func handleLspDefinition(deps *Deps) server.ToolHandler {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        var p LspDefinitionParams
        if err := req.BindArguments(&p); err != nil {
            return nil, err
        }
        mgr, err := deps.Router.Resolve(ctx, p.Path)
        if err != nil {
            return mcp.NewToolResultError(fmt.Sprintf("no language server for %s", p.Path)), nil
        }
        content, err := os.ReadFile(p.Path)
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }
        mgr.EnsureOpen(pathToURI(p.Path), string(content))

        locations, err := mgr.Definition(ctx, protocol.DefinitionParams{
            TextDocumentPositionParams: protocol.TextDocumentPositionParams{
                TextDocument: protocol.TextDocumentIdentifier{URI: pathToURI(p.Path)},
                Position:     protocol.Position{Line: uint32(p.Line - 1), Character: uint32(p.Character - 1)},
            },
        })
        if err != nil {
            return mcp.NewToolResultError(err.Error()), nil
        }

        return mcp.NewToolResultJSON(LspDefinitionResponse{
            Definitions: convertLocations(locations, true /* with preview */),
        }), nil
    }
}
```

Key design choices:
- **1-indexed everywhere** in the tool surface. LLMs reason about line numbers naturally as 1-indexed (since that's how editors display them). LSP internally is 0-indexed; we convert at the tool boundary.
- **Source line previews** included in every location. A bare `file.ts:47:12` is useless to an LLM that then has to Read the file to understand it. Including the source line for each location ("`const users = fetchUsers()`") lets the model reason in one step.
- **Never error on "unsupported language"**. Return a helpful error result instead. Tool errors should be recoverable information for the model, not crashes.

#### 6.3.2 `lspReferences`

Same shape as `lspDefinition` but calls `textDocument/references` and returns `ReferencesResponse`. Supports `include_declaration: bool` in params.

Bonus: group references by file and include a count so the model gets a summary first:

```json
{
  "total": 47,
  "by_file": [
    {"path": "src/api/users.ts", "count": 12},
    {"path": "src/components/UserList.tsx", "count": 8},
    {"path": "tests/users.test.ts", "count": 27}
  ],
  "references": [
    {"path": "src/api/users.ts", "line": 15, "column": 8, "preview": "  const users = fetchUsers();"},
    // ... all 47 ...
  ]
}
```

Volume cap: if `total > 100`, truncate the `references` array to the first 100 and set `truncated: true, omitted: 47`. The grouped summary is always complete.

#### 6.3.3 `lspHover`

Returns LSP `Hover` content. Strip markdown fences if the LSP server returned markdown; keep the text content but preserve code blocks. Most interesting for TypeScript (full inferred types) and Rust (rust-analyzer inlay hint info).

```json
{
  "type_signature": "function fetchUsers(): Promise<User[]>",
  "documentation": "Fetches the list of active users from the API.",
  "range": {"start": {"line": 15, "column": 8}, "end": {"line": 15, "column": 18}}
}
```

#### 6.3.4 `lspWorkspaceSymbol`

Fuzzy search across every symbol the language server's index knows about.

```json
{
  "query": "fetchUser",
  "symbols": [
    {
      "name": "fetchUsers",
      "kind": "function",
      "container": "src/api/users.ts",
      "path": "src/api/users.ts",
      "line": 12,
      "column": 8
    },
    {
      "name": "fetchUserById",
      "kind": "function",
      "container": "src/api/users.ts",
      "path": "src/api/users.ts",
      "line": 45,
      "column": 8
    }
  ]
}
```

Volume cap at 100 symbols, sorted by relevance (the LSP server's ordering).

#### 6.3.5 `lspDocumentSymbol`

Returns the outline of a file. Recursive hierarchy preserved.

```json
{
  "path": "src/api/users.ts",
  "symbols": [
    {
      "name": "UserService",
      "kind": "class",
      "line": 8, "column": 1,
      "end_line": 78, "end_column": 1,
      "children": [
        {"name": "constructor", "kind": "method", "line": 10, "column": 3, ...},
        {"name": "fetchUsers", "kind": "method", "line": 15, "column": 3, ...},
        {"name": "fetchUserById", "kind": "method", "line": 45, "column": 3, ...}
      ]
    }
  ]
}
```

#### 6.3.6 `lspCodeActions`

Most powerful tool for self-correction. Returns the quick-fixes and refactorings the language server offers at a given position.

```json
{
  "path": "src/api/users.ts",
  "actions": [
    {
      "title": "Add missing import 'User' from './types'",
      "kind": "quickfix",
      "is_preferred": true,
      "edit": {
        "changes": [
          {"path": "src/api/users.ts", "range": {...}, "new_text": "import { User } from './types';\n"}
        ]
      }
    },
    {
      "title": "Create type 'User'",
      "kind": "quickfix",
      "is_preferred": false,
      "command": {"title": "Create type", "command": "typescript.createType", "arguments": [...]}
    }
  ]
}
```

The `edit` field is a Droid-consumable edit description. The model applies it via Droid's own `Edit` or `apply-patch` tools — this preserves Droid's permission flow and undo history. For multi-file actions (common with refactoring quick-fixes), the model walks the `changes` array and applies each file's edits through Droid's normal tool flow. `lspd` does not apply edits itself; it only tells the model what the language server suggests.

#### 6.3.7 `lspRename`

Cross-file rename. Supports `dry_run: true` to preview the edit without applying it.

```json
{
  "old_name": "fetchUsers",
  "new_name": "getAllUsers",
  "files_touched": 8,
  "total_edits": 23,
  "edit": {
    "changes": [
      {"path": "src/api/users.ts", "range": {...}, "new_text": "getAllUsers"},
      // ... 22 more ...
    ]
  },
  "dry_run": true
}
```

Always call `lspRename` with `dry_run: true` first to review the plan. To execute, set `dry_run: false` and the model walks the returned `edit.changes` array and applies each file's edits via Droid's normal `Edit` or `apply-patch` tools. `lspd` never applies the edit itself — this keeps the permission flow and undo history in Droid's hands and prevents cross-file changes from happening silently in a single tool call.

#### 6.3.8 `lspFormat`

```json
{
  "path": "src/api/users.ts",
  "range": null,             // null = whole file; otherwise range
  "new_text": "<formatted content>",
  "changed": true
}
```

The caller can diff `new_text` against the current file to see what changed.

#### 6.3.9 `lspCallHierarchy`

Two-step because LSP's call hierarchy is two-step: `prepareCallHierarchy` returns items, then `incomingCalls` / `outgoingCalls` returns calls for each item.

```json
{
  "item": {"name": "fetchUsers", "kind": "function", "path": "src/api/users.ts", "line": 15},
  "direction": "incoming",
  "calls": [
    {
      "from": {"name": "UserList.render", "kind": "method", "path": "src/components/UserList.tsx", "line": 34},
      "call_sites": [{"path": "src/components/UserList.tsx", "line": 38, "column": 14}]
    },
    {
      "from": {"name": "loadUsers", "kind": "function", "path": "src/hooks/useUsers.ts", "line": 12},
      "call_sites": [{"path": "src/hooks/useUsers.ts", "line": 18, "column": 18}]
    }
  ]
}
```

#### 6.3.10 `lspTypeHierarchy`

Same two-step pattern as call hierarchy but for type relationships.

```json
{
  "item": {"name": "EventEmitter", "kind": "class", "path": "src/lib/events.ts"},
  "direction": "sub",
  "types": [
    {"name": "UserEventEmitter", "kind": "class", "path": "src/api/users.ts", "line": 5},
    {"name": "AuthEventEmitter", "kind": "class", "path": "src/api/auth.ts", "line": 8}
  ]
}
```

### 6.4 Tool descriptions

Tool descriptions are load-bearing for model behavior — they're what teach the model when to reach for which tool. These live in `internal/mcp/descriptions/` as constants that are easy to tune independently of the handler code.

Example:

```go
// Package descriptions holds the MCP tool descriptions. These strings are
// load-bearing: they teach the model when to choose each tool. Changes here
// should be tested against representative traces before landing.
package descriptions

const LspReferences = `Find every reference to a symbol across the entire project, using the language server's semantic index.

Use this tool instead of Grep whenever you need to find all call sites of a function, all places a variable is used, all subclasses of a class, or all references to an identifier. It returns the exact locations (file, line, column) that the language's compiler considers references, which is strictly more accurate than text search because it:

- Handles imports, aliases, and re-exports correctly
- Respects method overrides and interface implementations
- Ignores comments, strings, and unrelated symbols with the same name
- Works across module boundaries without you specifying paths

When to prefer lspReferences over Grep:
- Any refactor or rename preparation: "what calls this function?"
- Impact analysis: "what code depends on this type?"
- Understanding a symbol's usage before modifying it
- Finding subclasses or interface implementations

When Grep is still the right choice:
- Searching for text patterns or string literals
- Looking for TODO comments or error messages
- Finding files by extension or name
- Working in a language where no language server is configured

The tool returns the top 100 references by default, with a grouped summary showing how many references exist in each file. If more than 100 matches are found, you'll see truncated: true — consider narrowing by file if you need the full list.`

const LspCodeActions = `List the quick-fixes, refactorings, and source actions the language server offers at a specific position in a file.

Use this tool when you've hit an error and want to know if the language server has a canonical fix ready to go, rather than guessing at the fix yourself. Quick-fixes from language servers are usually correct: they know the project's types, imports, and conventions.

Common quick-fix categories:
- "Add missing import" — automatically adds the correct import statement with the right path
- "Create missing variable/function/type" — generates a stub the error is asking for
- "Fix typo" — replaces with the closest matching identifier the LSP knows about
- "Convert to ..." — converts between equivalent syntax (arrow function, destructuring, etc.)
- "Extract function/variable" — refactors a block into a named unit
- "Add missing return type annotation" — infers and inserts the type

After calling lspCodeActions, read the action's edit field and apply the changes via Droid's Edit or apply-patch tool. For multi-file actions (common with refactoring quick-fixes), walk the edit.changes array and apply each file's edits through Droid's normal tool flow so permission prompts and undo history work as expected.

Always prefer lspCodeActions over manually writing imports or fixes — the language server knows the project's module structure and will use the correct import path, re-export alias, and relative depth.`
```

These descriptions are tuned against real model traces to make sure the model actually reaches for them. Initial drafts go in with the implementation; later phases include a measurement loop where we trace model choices and refine the descriptions based on what the model actually does.

---

## 7. Unix socket server

### 7.1 Socket path and protocol

- Path: `~/.factory/run/lspd.sock`
- Protocol: line-delimited JSON. Each request is a single JSON object terminated by `\n`. Each response is a single JSON object terminated by `\n`.
- One request per connection, then close. Simple and debuggable.

### 7.2 Operations

| op | Request fields | Response fields | Behavior |
|---|---|---|---|
| `drain` | `path`, `session_id`, `kind` (`"edit"`/`"read"`), `timeout_ms`, `min_severity?` | `diagnostics`, `total`, `truncated`, `version` | Main read-hook path. For `kind="read"`: peek without reparse. For `kind="edit"`: push `didChange` if content differs, wait up to `timeout_ms` for publish. Apply policy layer. |
| `peek` | `path` | `diagnostics`, `version`, `updated_at` | Pure read from store. No reparse, no wait, no side effects. |
| `forget` | `session_id` | `ok` | Drop per-session dedup state. Called by SessionEnd hook. |
| `status` | — | `languages`, `sessions`, `mcp_port`, `uptime`, `version` | Used by `lspd status` and debugging. |
| `ping` | — | `ok`, `pid`, `version` | Liveness check for the launcher wrapper. |
| `reload` | — | `ok`, `changes` | Trigger config reload (same as SIGHUP). |

### 7.3 Authentication

None on the socket. Unix socket permission 0700 on the parent directory and 0600 on the socket file itself means only the same user can connect. No password, no token — filesystem permissions are the auth.

### 7.4 Hook client binary

`cmd/lsp-read-hook/main.go`:

```go
package main

import (
    "encoding/json"
    "fmt"
    "net"
    "os"
    "path/filepath"
    "strings"
    "time"
)

type hookInput struct {
    SessionID string                 `json:"session_id"`
    ToolName  string                 `json:"tool_name"`
    ToolInput map[string]interface{} `json:"tool_input"`
}

type hookOutput struct {
    HookSpecificOutput struct {
        HookEventName     string `json:"hookEventName"`
        AdditionalContext string `json:"additionalContext"`
    } `json:"hookSpecificOutput"`
    SuppressOutput bool `json:"suppressOutput"`
}

type socketReq struct {
    Op          string `json:"op"`
    Path        string `json:"path"`
    SessionID   string `json:"session_id"`
    Kind        string `json:"kind"`
    TimeoutMs   int    `json:"timeout_ms"`
    MinSeverity string `json:"min_severity,omitempty"`
}

type socketResp struct {
    Diagnostics []Diagnostic `json:"diagnostics"`
    Total       int          `json:"total"`
    Truncated   bool         `json:"truncated"`
}

type Diagnostic struct {
    Line     int    `json:"line"`
    Column   int    `json:"column"`
    Severity string `json:"severity"` // "error" | "warning" | ...
    Code     string `json:"code,omitempty"`
    Source   string `json:"source,omitempty"`
    Message  string `json:"message"`
}

func main() {
    var input hookInput
    if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
        os.Exit(0) // silent no-op on bad input
    }

    if input.ToolName != "Read" {
        os.Exit(0)
    }

    path, ok := input.ToolInput["file_path"].(string)
    if !ok || path == "" {
        os.Exit(0)
    }

    sockPath := filepath.Join(os.Getenv("HOME"), ".factory", "run", "lspd.sock")
    if _, err := os.Stat(sockPath); err != nil {
        os.Exit(0) // daemon not running, silent no-op
    }

    conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
    if err != nil {
        os.Exit(0)
    }
    defer conn.Close()
    conn.SetDeadline(time.Now().Add(2 * time.Second))

    req := socketReq{
        Op:        "drain",
        Path:      path,
        SessionID: input.SessionID,
        Kind:      "read",
        TimeoutMs: 1000,
    }
    if err := json.NewEncoder(conn).Encode(req); err != nil {
        os.Exit(0)
    }

    var resp socketResp
    if err := json.NewDecoder(conn).Decode(&resp); err != nil {
        os.Exit(0)
    }

    if len(resp.Diagnostics) == 0 {
        os.Exit(0)
    }

    ctx := formatContext(path, resp)
    out := hookOutput{SuppressOutput: true}
    out.HookSpecificOutput.HookEventName = "PostToolUse"
    out.HookSpecificOutput.AdditionalContext = ctx

    json.NewEncoder(os.Stdout).Encode(out)
}

func formatContext(path string, resp socketResp) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "<system-reminder>\nLSP diagnostics for %s:\n", filepath.Base(path))

    var errors, warnings []Diagnostic
    for _, d := range resp.Diagnostics {
        if d.Severity == "error" {
            errors = append(errors, d)
        } else if d.Severity == "warning" {
            warnings = append(warnings, d)
        }
    }
    if len(errors) > 0 {
        sb.WriteString("Errors:\n")
        for _, e := range errors {
            fmt.Fprintf(&sb, "  - Line %d: %s", e.Line, e.Message)
            if e.Source != "" {
                fmt.Fprintf(&sb, " (%s)", e.Source)
            }
            sb.WriteString("\n")
        }
    }
    if len(warnings) > 0 {
        if len(errors) > 0 {
            sb.WriteString("\n")
        }
        sb.WriteString("Warnings:\n")
        for _, w := range warnings {
            fmt.Fprintf(&sb, "  - Line %d: %s", w.Line, w.Message)
            if w.Source != "" {
                fmt.Fprintf(&sb, " (%s)", w.Source)
            }
            sb.WriteString("\n")
        }
    }
    if resp.Truncated {
        fmt.Fprintf(&sb, "\n(%d total; %d shown)\n", resp.Total, len(resp.Diagnostics))
    }
    sb.WriteString("</system-reminder>")
    return sb.String()
}
```

Total: ~130 lines. Fast cold start (Go is ~10ms), always exits 0, never blocks Droid.

---

## 8. Policy layer

Pure functions over `[]Diagnostic`. Tested in isolation with golden fixtures.

### 8.1 Dedup

```go
type Policy struct {
    cfg        *config.Policy
    sessions   *sync.Map // sessionID → *sessionState
    globalDedup *boltdb.DB // optional, for dedupe_scope=global
}

type sessionState struct {
    delivered map[string]time.Time // fingerprint → deliveredAt
    mu        sync.Mutex
}

func fingerprint(uri string, d protocol.Diagnostic) string {
    h := sha256.New()
    fmt.Fprintf(h, "%s\x00%d\x00%d\x00%v\x00%s\x00%s",
        uri,
        d.Range.Start.Line, d.Range.Start.Character,
        d.Code,
        d.Source,
        d.Message,
    )
    return hex.EncodeToString(h.Sum(nil))
}

func (p *Policy) Apply(sessionID, uri string, diags []protocol.Diagnostic) []protocol.Diagnostic {
    // 1. Severity filter
    filtered := p.filterSeverity(diags)

    // 2. Source filter
    filtered = p.filterSources(filtered)

    // 3. Dedup
    if p.cfg.DedupeScope != "none" {
        filtered = p.dedupFilter(sessionID, uri, filtered)
    }

    // 4. Sort by severity then line
    sort.SliceStable(filtered, func(i, j int) bool {
        if filtered[i].Severity != filtered[j].Severity {
            return filtered[i].Severity < filtered[j].Severity // 0=Error first
        }
        return filtered[i].Range.Start.Line < filtered[j].Range.Start.Line
    })

    // 5. Volume cap per file
    if len(filtered) > p.cfg.MaxPerFile {
        filtered = filtered[:p.cfg.MaxPerFile]
    }

    return filtered
}

func (p *Policy) dedupFilter(sessionID, uri string, diags []protocol.Diagnostic) []protocol.Diagnostic {
    if sessionID == "" {
        return diags
    }
    sv, _ := p.sessions.LoadOrStore(sessionID, &sessionState{delivered: make(map[string]time.Time)})
    st := sv.(*sessionState)
    st.mu.Lock()
    defer st.mu.Unlock()

    out := diags[:0]
    for _, d := range diags {
        fp := fingerprint(uri, d)
        if _, seen := st.delivered[fp]; seen {
            continue
        }
        st.delivered[fp] = time.Now()
        out = append(out, d)
    }
    return out
}
```

### 8.2 Volume caps

Applied in two places:

- **Per file** inside `Policy.Apply` (above).
- **Per turn** at the delivery layer (MCP or socket handler): track a running total for the current request and stop adding once `max_per_turn` is reached. Set `truncated: true, total: N` in the response.

### 8.3 Severity / source filter

```go
func (p *Policy) filterSeverity(diags []protocol.Diagnostic) []protocol.Diagnostic {
    min := p.cfg.MinSeverity // e.g., "warning" → severity ≤ 1
    out := diags[:0]
    for _, d := range diags {
        if int(d.Severity) <= min {
            out = append(out, d)
        }
    }
    return out
}

func (p *Policy) filterSources(diags []protocol.Diagnostic) []protocol.Diagnostic {
    if len(p.cfg.SourceAllowlist) == 0 && len(p.cfg.SourceDenylist) == 0 {
        return diags
    }
    out := diags[:0]
    for _, d := range diags {
        src := d.Source
        if len(p.cfg.SourceAllowlist) > 0 {
            allowed := false
            for _, a := range p.cfg.SourceAllowlist {
                if src == a {
                    allowed = true
                    break
                }
            }
            if !allowed {
                continue
            }
        }
        denied := false
        for _, d2 := range p.cfg.SourceDenylist {
            if src == d2 {
                denied = true
                break
            }
        }
        if denied {
            continue
        }
        out = append(out, d)
    }
    return out
}
```

### 8.4 Code action attachment

When `attach_code_actions: true`, for each diagnostic that survives filtering, issue a `textDocument/codeAction` request with the diagnostic in `context.diagnostics` and `only: ["quickfix"]`. Take the first action (if any) and attach its title to the diagnostic as a `quick_fix_preview` field:

```json
{
  "line": 12,
  "column": 8,
  "severity": "error",
  "code": "2304",
  "source": "ts",
  "message": "Cannot find name 'fetchUsers'",
  "quick_fix_preview": "Add import 'fetchUsers' from './api'"
}
```

The system-reminder formatter includes the preview on a second line:

```
Errors:
  - Line 12: Cannot find name 'fetchUsers' (ts)
    quick-fix: Add import 'fetchUsers' from './api'
```

Code-action queries are fired in parallel (goroutine per diagnostic) with a short timeout (300ms per action). If a query times out or errors, we silently skip it for that diagnostic. This is best-effort context augmentation, not a hard requirement.

---

## 9. File watcher

`internal/watcher` uses `github.com/fsnotify/fsnotify` to watch every project root the daemon has seen.

- On first touch of a file under a new root, add the root to the watcher.
- On `fsnotify.Write` events for files with recognized extensions, dispatch a `didChange` to the relevant `ClientManager` using the file's current content.
- Debounce: 200ms coalesce window per file (editors often write multiple times in quick succession).
- Skip directories in `.gitignore`, `node_modules`, `.git`, `.venv`, `dist`, `build`, `target`.
- Skip files larger than 2 MB to avoid thrashing on generated code.

This keeps the diagnostic store fresh even when the user edits in their own editor outside of Droid, so the next `Read` or `Edit` in Droid sees accurate diagnostics.

---

## 10. Configuration

### 10.1 Schema

`~/.factory/hooks/lsp/lspd.yaml`. Project overrides at `<project>/.factory/lsp/lspd.yaml` are merged on top (project wins for overlapping keys).

### 10.2 Full example

```yaml
# ~/.factory/hooks/lsp/lspd.yaml

daemon:
  # Filesystem paths
  run_dir: ~/.factory/run
  log_file: ~/.factory/logs/lspd.log
  config_dir: ~/.factory/hooks/lsp

  # HTTP MCP server
  http_host: 127.0.0.1
  http_port: 0            # 0 = auto-assign

  # Unix socket
  socket_path: ~/.factory/run/lspd.sock

  # Lifecycle
  idle_timeout: 30m
  shutdown_grace: 5s

  # Logging
  log_level: info         # debug | info | warn | error
  log_format: json        # json | text
  log_max_size_mb: 50
  log_max_backups: 5
  log_max_age_days: 7

  # Metrics (opt-in)
  metrics_addr: ""        # "127.0.0.1:9464" to enable

policy:
  min_severity: warning
  max_diagnostics_per_file: 20
  max_diagnostics_per_turn: 50
  dedupe_scope: session           # none | session | global
  dedupe_ttl: 24h                 # for global scope only
  attach_code_actions: true
  code_action_timeout: 300ms

  source_allowlist: []            # empty = all allowed
  source_denylist:
    - eslint-plugin-import        # known noisy source

  severity_overrides:
    pyright:
      reportMissingTypeStubs: hint  # demote noisy pyright diagnostic

languages:
  typescript:
    command: typescript-language-server
    args: [--stdio]
    extensions: [.ts, .tsx, .js, .jsx, .mjs, .cjs]
    root_markers: [tsconfig.json, jsconfig.json, package.json]
    warmup: true
    max_restarts: 5
    restart_window: 10m
    env:
      NODE_OPTIONS: --max-old-space-size=4096
    initialization_options:
      preferences:
        includeInlayParameterNameHints: none
        includeInlayVariableTypeHints: false
        includeInlayFunctionParameterTypeHints: false
    settings:
      typescript:
        tsserver:
          maxTsServerMemory: 4096
      javascript:
        implicitProjectConfig:
          checkJs: true

  python:
    command: pyright-langserver
    args: [--stdio]
    extensions: [.py, .pyi]
    root_markers: [pyrightconfig.json, pyproject.toml, setup.py, setup.cfg]
    warmup: true
    max_restarts: 5
    settings:
      python:
        analysis:
          typeCheckingMode: basic
          autoSearchPaths: true
          useLibraryCodeForTypes: true
          diagnosticMode: openFilesOnly
        pythonPath: python3

  go:
    command: gopls
    args: [serve]
    extensions: [.go]
    root_markers: [go.mod, go.work]
    warmup: true
    settings:
      gopls:
        staticcheck: true
        gofumpt: true
        usePlaceholders: false
        hints:
          assignVariableTypes: false

  rust:
    command: rust-analyzer
    args: []
    extensions: [.rs]
    root_markers: [Cargo.toml, Cargo.lock]
    warmup: false        # rust-analyzer indexing is heavy
    max_restarts: 3
    settings:
      rust-analyzer:
        cargo:
          allFeatures: false
        check:
          command: check

  cpp:
    command: clangd
    args: [--background-index, --header-insertion=never, --clang-tidy]
    extensions: [.c, .cc, .cpp, .cxx, .h, .hpp, .hxx]
    root_markers: [compile_commands.json, CMakeLists.txt, .clangd]

  lua:
    command: lua-language-server
    args: []
    extensions: [.lua]
    root_markers: [.luarc.json, .luarc.jsonc, stylua.toml]

  # Disabled languages: configured but not started until a file is touched
  # Lazy spawn is the default for all languages. Use `disabled: true` to
  # prevent a language from ever starting even if a file is touched.
  java:
    disabled: true

mcp:
  tier1_enabled: true
  tier2_enabled: true
  tools:
    # Explicit enable/disable list per tool. Defaults to all enabled.
    lspCallHierarchy: true
    lspTypeHierarchy: true
    lspRename: true
    lspFormat: true
  session_header: X-Droid-Session-Id
```

### 10.3 Per-project overrides

A project's `.factory/lsp/lspd.yaml` merges on top. Common use case: override a language's command or settings for one specific project.

```yaml
# project/.factory/lsp/lspd.yaml
languages:
  python:
    command: uv
    args: [run, pyright-langserver, --stdio]
    settings:
      python:
        analysis:
          typeCheckingMode: strict
```

Merging semantics: deep merge for maps, replace for lists (simplifies the mental model — "does this project want a full list override, or just a patch?" is solved by always treating lists as replace).

---

## 11. Droid integration

Four integration points. Each has a distinct purpose; all four together give full coverage.

### 11.1 Launcher wrapper (primary path)

Installed as `~/.local/bin/droid` ahead of the real `droid` in `$PATH`:

```sh
#!/bin/sh
# ~/.local/bin/droid — lspd-aware launcher for Droid

set -eu

LSPD="${LSPD_BIN:-lspd}"
REAL_DROID="${REAL_DROID:-/Applications/Droid.app/Contents/MacOS/droid}"

# Ensure lspd is running (idempotent)
if ! "$LSPD" ping >/dev/null 2>&1; then
    if ! "$LSPD" start --quiet; then
        echo "[droid-launcher] warning: lspd failed to start; continuing without LSP bridge" >&2
    fi
fi

# Export the chosen MCP port
PORT_FILE="$HOME/.factory/run/lspd.port"
if [ -f "$PORT_FILE" ]; then
    FACTORY_VSCODE_MCP_PORT="$(cat "$PORT_FILE")"
    export FACTORY_VSCODE_MCP_PORT
fi

exec "$REAL_DROID" "$@"
```

This is the primary path because setting `FACTORY_VSCODE_MCP_PORT` *before* `exec droid` sidesteps the timing question entirely — `IdeContextManager` sees the variable at construction time.

### 11.2 SessionStart hook (fallback path)

For users who don't install the launcher wrapper. Less clean than the wrapper because of the `IdeContextManager` timing question (§18), but worth having as a fallback.

`.factory/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "\"$FACTORY_PROJECT_DIR\"/.factory/hooks/lsp/session-start.sh",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

`session-start.sh`:

```sh
#!/bin/sh
# Ensures lspd is up, writes FACTORY_VSCODE_MCP_PORT to $CLAUDE_ENV_FILE,
# and warms up language servers for the current project.

set -eu

if ! command -v lspd >/dev/null 2>&1; then
    exit 0
fi

if ! lspd ping >/dev/null 2>&1; then
    lspd start --quiet || exit 0
fi

PORT="$(cat "$HOME/.factory/run/lspd.port" 2>/dev/null || echo '')"
if [ -n "$PORT" ] && [ -n "${CLAUDE_ENV_FILE:-}" ]; then
    echo "export FACTORY_VSCODE_MCP_PORT=$PORT" >> "$CLAUDE_ENV_FILE"
fi

# Async warmup hint (non-blocking)
lspd warmup --cwd "$cwd" >/dev/null 2>&1 &

# Emit a minimal SessionStart additionalContext so the model knows lspd is up
cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "LSP bridge active: semantic code navigation tools (lspDefinition, lspReferences, lspHover, lspWorkspaceSymbol, lspDocumentSymbol, lspCodeActions, lspRename, lspFormat, lspCallHierarchy, lspTypeHierarchy) are available as IDE-native tools. Diagnostics are automatically injected after Read, Edit, Create, and Write."
  }
}
EOF
```

### 11.3 PostToolUse Read hook (Read gap filler)

`.factory/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Read",
        "hooks": [
          {
            "type": "command",
            "command": "\"$HOME\"/.local/bin/lsp-read-hook",
            "timeout": 3
          }
        ]
      }
    ]
  }
}
```

`lsp-read-hook` is the Go binary from §7.4. It reads hook JSON, queries the daemon, emits `hookSpecificOutput.additionalContext`, exits 0. Always 0, never blocks Droid.

### 11.4 SessionEnd hook

`.factory/settings.json`:

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "lspd forget --session \"$session_id\"",
            "timeout": 2
          }
        ]
      }
    ]
  }
}
```

Drops per-session dedup state. Daemon keeps running for the next session.

---

## 12. Data flow walkthroughs

### 12.1 Edit flow (end-to-end)

1. Model emits `Edit` tool call with `file_path: src/api/users.ts`, `old_str: ...`, `new_str: ...`.
2. Droid's `edit-cli.ts` (`executors/client/edit-cli.ts:104`) calls `fetchDiagnostics(deps.ideClient, filePath)`.
3. `fetchDiagnostics` (`file-tools/diagnostics-utils.ts:38`) calls `ideClient.callTool('getIdeDiagnostics', { uri: 'file:///path/to/src/api/users.ts' })`.
4. `VSCodeIdeClient.callTool` sends the MCP `tools/call` request over StreamableHTTP to `http://127.0.0.1:<port>/mcp`.
5. `lspd`'s MCP server routes to `internal/mcp/tools/compat.handleGetIdeDiagnostics`.
6. Handler resolves the path through the router. Since this is the first touch of `src/api/users.ts`:
   - Router finds `typescript` for `.ts` extension.
   - Router checks if a `ClientManager` exists for `typescript`. None yet.
   - Router lazily spawns one: fork `typescript-language-server --stdio`, send `initialize` with workspace folder `<project root>` derived from `tsconfig.json`.
   - Warmup: empty `workspace/symbol` query.
   - Manager is ready (~2–5 seconds cold, once per daemon lifetime).
   - Router caches the manager.
7. Handler reads the file contents, calls `mgr.EnsureOpen(uri, content)` → sends `didOpen` → tsserver starts analyzing the file.
8. Handler calls `deps.Store.Wait(uri, 0, 1200ms)`. tsserver publishes diagnostics ~500ms later. Store signals the waiter. Handler gets the current snapshot.
9. Policy layer: severity filter (drops info/hint), source filter, dedup against empty session state (this is the first call in the session), volume cap, sort.
10. Handler returns JSON `{"diagnostics": [...]}` to the MCP server → serialized over StreamableHTTP → `VSCodeIdeClient` receives → `fetchDiagnostics` returns `diagnosticsBefore`.
11. `edit-cli.ts` runs the actual edit via `editFileWithDiff`. File is modified on disk.
12. `edit-cli.ts` calls `fetchDiagnostics` again, this time with `maxRetries=1, delayMs=500` (`diagnostics-utils.ts:144`).
13. Inside the second call, the MCP handler sees the file is already tracked. It reads the new file contents, calls `mgr.EnsureOpen` which detects the content changed and sends `didChange` with incremented version.
14. tsserver reparses. Handler waits up to 1200ms for the publish. Publish arrives with new diagnostics reflecting the edit.
15. Policy layer runs. Dedup sees that pre-edit errors are already in the session's delivered set — they're filtered out. New errors introduced by the edit pass through.
16. Handler returns. `fetchDiagnostics` returns `diagnosticsAfter`.
17. `edit-cli.ts` runs `compareDiagnostics(diagnosticsBefore, diagnosticsAfter)` → returns only brand-new errors.
18. `edit-cli.ts` runs `formatDiagnosticsForSystemReminder` → produces `<system-reminder>New errors detected after editing users.ts: ...`.
19. `edit-cli.ts` attaches the system-reminder string to the tool result value.
20. Tool result returns up through `ToolExecutor.ts` → PostToolUse hooks fire (including our Read hook, but it doesn't match `Edit` so it's a no-op) → result flows to agent loop.
21. Agent's next LLM call includes the tool result with the diagnostics embedded. Model sees them before planning its next step.

Timing budget (warm daemon, warm LSP, medium-sized TS project):

| Step | Target |
|---|---|
| `fetchDiagnostics` (before) — warm | <50ms |
| `editFileWithDiff` | 20ms |
| `fetchDiagnostics` (after) — `didChange` + publish + 1 retry | 200–800ms |
| Policy layer | <10ms |
| Total added latency vs. no lspd | ~300–900ms per edit |

Cold first-edit (LSP server not yet spawned): add 2–5 seconds for `initialize` + warmup. Happens once per daemon lifetime per language.

### 12.2 Create flow

Same as edit flow except step 2 is skipped (no `diagnosticsBefore` — there's nothing before a create). `create-cli.ts:191` calls `fetchDiagnostics` once after creation. Everything else identical.

### 12.3 Read flow

1. Model emits `Read` tool call with `file_path: src/api/users.ts`.
2. Droid's `read-cli.ts` reads the file contents. Returns `{ content: ..., ... }`.
3. `ToolExecutor.ts:1385` awaits `executeHooksWithDisplay(PostToolUse, ...)`.
4. The Read matcher hits our hook. `lsp-read-hook` is spawned with hook JSON on stdin.
5. `lsp-read-hook` reads stdin, extracts `file_path` and `session_id`.
6. Connects to `~/.factory/run/lspd.sock`. Sends `{"op": "drain", "path": "...", "session_id": "...", "kind": "read", "timeout_ms": 1000}`.
7. `lspd` socket handler resolves the path → router finds the language manager (lazy spawn if needed, same as edit flow).
8. Because `kind=read`, the handler *does not* send `didChange` — the file on disk is what tsserver is already tracking. It peeks the store for the current entry.
9. If the store has nothing, the handler does a single `didOpen` (or fresh `didChange` if the file was modified out-of-band) and waits briefly. For routine reads of files tsserver already knows about, this is a pure peek with <5ms latency.
10. Policy layer runs. Dedup against the session state — previously-delivered diagnostics are filtered out.
11. Response serialized back to `lsp-read-hook`.
12. `lsp-read-hook` formats as `<system-reminder>`, emits `hookSpecificOutput.additionalContext`, exits 0.
13. `ToolExecutor.ts:1420` collects the additionalContext → dispatches `ADD_MESSAGE` with System role → returns tool result.
14. Agent's next LLM call sees the Read result AND the System message with diagnostics.

Timing: <100ms for a peek of a tracked file. If the file isn't tracked yet, +200–500ms for first `didOpen`.

### 12.4 LSP crash mid-session

1. `typescript-language-server` crashes (OOM, bug, etc.).
2. `ClientManager.Wait` returns with the exit code.
3. Supervisor goroutine observes the exit. Logs the error. Enters restart logic.
4. First restart attempt after 1s backoff. Spawns new tsserver subprocess, re-runs `initialize`, re-sends `didChangeConfiguration`, re-issues `didOpen` for every tracked document (since tsserver has no state from before the crash).
5. New tsserver emits fresh `publishDiagnostics` for all re-opened files.
6. Supervisor marks the manager as healthy. Meanwhile, any in-flight requests from before the crash received an error from `ClientManager.Call`. Those requests return empty diagnostic lists to their callers — Droid edits don't fail, they just miss diagnostics for that one call.
7. Next `fetchDiagnostics` call sees a healthy manager. Works normally.

### 12.5 Daemon not running

1. Droid starts, `FACTORY_VSCODE_MCP_PORT` is not set (user didn't use the launcher wrapper and isn't using a SessionStart hook).
2. `IdeContextManager` sees no env var, does not connect. `ideClient` is `undefined` throughout the session.
3. `fetchDiagnostics(undefined, ...)` returns `[]` immediately (first check in `diagnostics-utils.ts:44`).
4. `compareDiagnostics([], [])` returns `[]`.
5. `formatDiagnosticsForSystemReminder([], ...)` returns `null`.
6. Tool result is returned without any system-reminder. Edit succeeds normally, no diagnostics visible.

Graceful degradation: when lspd is down, Droid works exactly as it did before lspd existed. No errors, no hangs, no surprises.

### 12.6 Daemon crashed after session started

1. Daemon crashes for some reason (OOM, supervisor bug).
2. `VSCodeIdeClient` in Droid has a live MCP connection that now errors on every call.
3. `fetchDiagnostics` catches the error and returns `[]` (line 64 of `diagnostics-utils.ts`).
4. System-reminder comes up empty. Edit proceeds normally.
5. Read hook: `lsp-read-hook` can't connect to the socket, exits 0. Read proceeds normally.

No Droid-visible failure. The user might notice diagnostics aren't appearing and investigate. `lspd status` will fail, pointing them to the problem.

---

## 13. Observability

### 13.1 Logging

Structured JSON logs via `log/slog` with rotation via `lumberjack`. One line per log event. Every line includes:

- `time` (RFC3339 nanos)
- `level` (debug | info | warn | error)
- `component` (daemon | lsp.client | lsp.supervisor | mcp | socket | policy | config | watcher)
- `message` (human-readable)
- Structured fields per event

Example lines:

```json
{"time":"2026-04-10T14:22:08.123Z","level":"info","component":"daemon","message":"lspd started","port":47293,"pid":41823,"version":"0.1.0"}
{"time":"2026-04-10T14:22:15.441Z","level":"info","component":"lsp.client","message":"spawned","language":"typescript","pid":41847,"root":"/Users/harsha/code/foo"}
{"time":"2026-04-10T14:22:15.892Z","level":"info","component":"lsp.client","message":"initialized","language":"typescript","duration_ms":451}
{"time":"2026-04-10T14:22:16.102Z","level":"debug","component":"lsp.client","message":"publish diagnostics","language":"typescript","uri":"file:///Users/harsha/code/foo/src/api/users.ts","count":3,"version":2}
{"time":"2026-04-10T14:22:16.110Z","level":"info","component":"mcp","message":"getIdeDiagnostics","uri":"file:///Users/harsha/code/foo/src/api/users.ts","raw_count":3,"filtered_count":2,"duration_ms":8}
{"time":"2026-04-10T14:22:16.118Z","level":"info","component":"policy","message":"dedup","session_id":"8f2eabc...","uri":"file:///...","before":3,"after":2,"removed":1}
```

Rotation: default 50 MB per file, 5 backups, 7 days retention. Configurable.

### 13.2 Metrics (optional, Prometheus)

When `daemon.metrics_addr` is set, exposes `/metrics` on that address. Exported metrics:

```
lspd_up                                                              # gauge, 0 or 1
lspd_lsp_up{language="typescript"}                                   # gauge
lspd_lsp_restarts_total{language="typescript"}                       # counter
lspd_lsp_request_duration_seconds{language,method}                   # histogram
lspd_lsp_request_errors_total{language,method}                       # counter
lspd_diagnostic_count{language,severity}                             # gauge
lspd_mcp_request_duration_seconds{tool}                              # histogram
lspd_mcp_request_errors_total{tool}                                  # counter
lspd_socket_request_duration_seconds{op}                             # histogram
lspd_socket_request_errors_total{op}                                 # counter
lspd_policy_dedup_hits_total                                         # counter
lspd_policy_volume_truncations_total                                 # counter
lspd_active_sessions                                                 # gauge
lspd_tracked_documents{language}                                     # gauge
lspd_idle_seconds                                                    # gauge
```

### 13.3 `lspd status` output

```
lspd 0.1.0   pid 41823   uptime 2h14m   idle 3m42s
MCP http://127.0.0.1:47293/mcp   socket ~/.factory/run/lspd.sock

Languages:
  typescript    pid 41847   uptime 2h13m   docs  47   ready     last publish 3s ago   restarts 0
  python        pid 41901   uptime 2h13m   docs  12   ready     last publish 1m12s ago restarts 0
  go            pid 42344   uptime 18m     docs   3   ready     last publish 2m03s ago restarts 1
  rust          —                                                 not started
  cpp           —                                                 not started

Sessions:
  8f2e…abc   started 1h42m ago   delivered 412 diagnostics   cwd /Users/harsha/code/foo
  3a91…def   started 14m ago     delivered  19 diagnostics   cwd /Users/harsha/code/bar

Policy:  min=warning  per_file=20  per_turn=50  dedupe=session  code_actions=on
```

### 13.4 Debug endpoint

When `daemon.metrics_addr` is set, also exposes `/debug/lspd` on the same port. Returns a JSON dump of internal state:

```json
{
  "version": "0.1.0",
  "uptime_seconds": 8048,
  "config_path": "/Users/harsha/.factory/hooks/lsp/lspd.yaml",
  "languages": {
    "typescript": {
      "state": "healthy",
      "pid": 41847,
      "root": "/Users/harsha/code/foo",
      "open_documents": 47,
      "last_publish": "2026-04-10T14:22:16.102Z",
      "request_count": 1842,
      "request_errors": 3,
      "restarts": 0,
      "document_list": ["file:///Users/harsha/code/foo/src/api/users.ts", ...]
    }
  },
  "sessions": [...],
  "diagnostic_store": {
    "entries": 47,
    "total_diagnostics": 128
  }
}
```

Gated by config; off by default.

---

## 14. Testing strategy

### 14.1 Unit tests (per package)

Every `internal/*` package has unit tests. Target coverage: 80% for logic-heavy packages (policy, lsp/store, lsp/client_manager, config), 60% for I/O-heavy packages.

- **`internal/policy`**: pure functions, tested with table-driven golden fixtures. Input: a JSON fixture of diagnostics plus policy config. Output: a JSON fixture of filtered diagnostics. Diffs are reviewed in PR.
- **`internal/lsp/store`**: race-heavy, tested with `go test -race`. Concurrent publishers, concurrent waiters, wait-then-publish, publish-then-wait, timeout paths.
- **`internal/lsp/client`**: tested against a mock LSP server (`mcptest`-style, we implement our own `internal/lsp/mocklsp`). Mock accepts JSON-RPC, responds according to a scripted scenario. Tests cover: successful initialize, initialize failure, didChange→publishDiagnostics round-trip, request timeout, notification handling.
- **`internal/config`**: table-driven YAML parsing and validation tests. Invalid configs should produce clear error messages.
- **`internal/lsp/router`**: project root detection tests using temp directories with marker files.

### 14.2 Integration tests

`integration_test.go` files that spawn real LSP servers in a `// +build integration` tag. Gated behind `make test-integration` because they require system tools (`typescript-language-server`, `pyright-langserver`, `gopls`) to be installed.

- **ts-e2e**: Start lspd, issue `getIdeDiagnostics` against a test TS project with a known error, assert the error is returned with the correct line/column/code/source.
- **py-e2e**: Same for Python and pyright.
- **go-e2e**: Same for Go and gopls.
- **rust-e2e**: Same for Rust and rust-analyzer (skipped in CI due to resource cost; runs on-demand).
- **navigation-e2e**: Call `lspDefinition`/`lspReferences`/`lspHover` against a test TS project, assert the returned locations match expected results from a manual tsserver session.
- **crash-recovery-e2e**: Spawn a supervisor with a deliberately-killable child, verify restart behavior.
- **read-hook-e2e**: Invoke `lsp-read-hook` binary with canned hook JSON and a running lspd, assert the output JSON has the expected additionalContext.

### 14.3 Golden tests for policy

Policy behavior is critical for context efficiency. Golden tests capture:

```
testdata/policy/
  001-simple-dedup/
    config.yaml
    input.json       # []Diagnostic before session state
    session.json     # Per-session delivered set before this call
    expected.json    # Filtered output
  002-volume-cap-per-file/
    ...
  003-severity-filter/
    ...
```

Each test loads the config, loads the session state, feeds input to `Policy.Apply`, and asserts the output matches `expected.json` byte-for-byte (with JSON canonicalization). Running `go test -update` regenerates all expected outputs from current behavior — useful when intentionally changing the policy.

### 14.4 MCP contract test

A small Go test that stands up `lspd` in-process, connects an MCP client to it (using `mcp-go`'s client package), and calls every tool with a canned request. Asserts the response shape matches what `VSCodeIdeClient` expects. This is the single most important test — if it passes, Droid integration will work.

### 14.5 Real Droid smoke test

Manual (or scripted) test run:

1. Start `lspd` in foreground with debug logging.
2. `export FACTORY_VSCODE_MCP_PORT=$(cat ~/.factory/run/lspd.port)`.
3. Run `droid --debug` in a test project with a known broken file (e.g., an unimported identifier).
4. Ask the model to "read src/api/users.ts and tell me what errors you see."
5. Verify the model reports the errors. Verify lspd logs show `getIdeDiagnostics` was called. Verify the Read hook fired and returned additionalContext.
6. Ask the model to "find all references to fetchUsers." Verify the model calls `lspReferences`. Verify the response matches expectations.
7. Break the file in an additional way (introduce a new error). Ask the model to read it again. Verify the new error is reported (dedup lets the new one through). Read it a second time. Verify no duplicate errors are reported (dedup filters the repeat).

This runs once per release as a gate.

---

## 15. File structure & LOC budget

```
droid-lsp/
├── PLAN.md                                         # this file
├── README.md                                       # user-facing overview (~200 LOC)
├── go.mod
├── go.sum
├── Makefile                                        # build, test, lint, install targets
├── .golangci.yml                                   # linter config
│
├── cmd/
│   ├── lspd/                                       # main daemon binary
│   │   ├── main.go                                 # entrypoint, subcommand dispatch (~100)
│   │   ├── start.go                                # start subcommand (~80)
│   │   ├── stop.go                                 # stop subcommand (~40)
│   │   ├── status.go                               # status subcommand (~120)
│   │   ├── reload.go                               # reload subcommand (~30)
│   │   ├── logs.go                                 # logs subcommand (~40)
│   │   ├── diag.go                                 # diag subcommand (~60)
│   │   ├── fix.go                                  # fix subcommand (~60)
│   │   ├── ping.go                                 # ping subcommand (~20)
│   │   └── daemonize.go                            # self-daemonize sentinel (~80)
│   └── lsp-read-hook/                              # PostToolUse Read hook binary
│       └── main.go                                 # ~130
│
├── internal/
│   ├── daemon/
│   │   ├── daemon.go                               # main loop, lifecycle (~200)
│   │   ├── lock.go                                 # pidfile / flock (~80)
│   │   ├── signals.go                              # SIGHUP/SIGTERM handling (~60)
│   │   └── idle.go                                 # idle timeout (~50)
│   │
│   ├── lsp/
│   │   ├── client/
│   │   │   ├── manager.go                          # ClientManager core (~350)
│   │   │   ├── handshake.go                        # initialize / initialized (~120)
│   │   │   ├── notify.go                           # notification routing (~100)
│   │   │   ├── requests.go                         # typed LSP method wrappers (~200)
│   │   │   └── docs.go                             # open document tracker (~120)
│   │   ├── router/
│   │   │   ├── router.go                           # extension routing, lazy spawn (~120)
│   │   │   └── rootdetect.go                       # project root walker (~60)
│   │   ├── supervisor/
│   │   │   └── supervisor.go                       # crash recovery, backoff (~150)
│   │   └── store/
│   │       └── store.go                            # diagnostic store with Wait (~200)
│   │
│   ├── mcp/
│   │   ├── server.go                               # mcp-go server setup (~120)
│   │   ├── session.go                              # session tracking (~80)
│   │   ├── tools/
│   │   │   ├── compat/
│   │   │   │   ├── diagnostics.go                  # getIdeDiagnostics (~200)
│   │   │   │   └── stubs.go                        # openDiff/closeDiff/openFile (~60)
│   │   │   └── nav/
│   │   │       ├── definition.go                   # lspDefinition (~120)
│   │   │       ├── references.go                   # lspReferences (~150)
│   │   │       ├── hover.go                        # lspHover (~100)
│   │   │       ├── workspace_symbol.go             # lspWorkspaceSymbol (~100)
│   │   │       ├── document_symbol.go              # lspDocumentSymbol (~120)
│   │   │       ├── code_actions.go                 # lspCodeActions (~180)
│   │   │       ├── rename.go                       # lspRename (~150)
│   │   │       ├── format.go                       # lspFormat (~100)
│   │   │       ├── call_hierarchy.go               # lspCallHierarchy (~160)
│   │   │       └── type_hierarchy.go               # lspTypeHierarchy (~140)
│   │   └── descriptions/
│   │       └── descriptions.go                     # all tool description strings (~400)
│   │
│   ├── socket/
│   │   ├── server.go                               # unix socket accept loop (~100)
│   │   ├── protocol.go                             # request/response types (~80)
│   │   └── handlers.go                             # drain/peek/forget/status/ping (~200)
│   │
│   ├── policy/
│   │   ├── policy.go                               # Apply entrypoint (~120)
│   │   ├── dedup.go                                # fingerprint + session state (~150)
│   │   ├── volume.go                               # per-file / per-turn caps (~60)
│   │   ├── severity.go                             # severity/source filters (~100)
│   │   └── code_actions.go                         # quick-fix attachment (~120)
│   │
│   ├── config/
│   │   ├── config.go                               # schema structs, defaults (~200)
│   │   ├── load.go                                 # YAML load + project merge (~150)
│   │   ├── validate.go                             # schema validation (~100)
│   │   └── reload.go                               # SIGHUP hot reload (~80)
│   │
│   ├── watcher/
│   │   └── watcher.go                              # fsnotify wrapper (~150)
│   │
│   ├── metrics/
│   │   └── metrics.go                              # Prometheus registry (~100)
│   │
│   ├── log/
│   │   └── log.go                                  # slog + lumberjack setup (~80)
│   │
│   └── format/
│       ├── system_reminder.go                      # diagnostics → <system-reminder> (~120)
│       └── lsp_to_llm.go                           # LSP types → LLM JSON shapes (~150)
│
├── scripts/
│   ├── install.sh                                  # install binary + wrapper + hooks
│   ├── droid-launcher.sh                           # ~/.local/bin/droid wrapper
│   └── session-start.sh                            # SessionStart hook script
│
├── examples/
│   ├── settings.json                               # example Droid hook config
│   └── lspd.yaml                                   # annotated config example
│
└── test/
    ├── unit/                                       # embedded in internal/*/\*_test.go
    ├── integration/                                # build-tag gated real-LSP tests
    │   ├── ts_test.go
    │   ├── py_test.go
    │   ├── go_test.go
    │   └── navigation_test.go
    ├── golden/                                     # policy golden fixtures
    │   └── policy/
    │       ├── 001-simple-dedup/
    │       ├── 002-volume-cap/
    │       └── ...
    ├── mocklsp/                                    # mock LSP server for unit tests
    │   └── mock.go
    └── e2e/                                        # MCP contract tests + Droid smoke tests
        ├── mcp_contract_test.go
        └── droid_smoke.md
```

### LOC budget

| Area | Estimated LOC |
|---|---|
| `cmd/lspd` | 630 |
| `cmd/lsp-read-hook` | 130 |
| `internal/daemon` | 390 |
| `internal/lsp/client` | 890 |
| `internal/lsp/router` | 180 |
| `internal/lsp/supervisor` | 150 |
| `internal/lsp/store` | 200 |
| `internal/mcp` | 200 |
| `internal/mcp/tools/compat` | 260 |
| `internal/mcp/tools/nav` | 1,320 |
| `internal/mcp/descriptions` | 400 |
| `internal/socket` | 380 |
| `internal/policy` | 550 |
| `internal/config` | 530 |
| `internal/watcher` | 150 |
| `internal/metrics` | 100 |
| `internal/log` | 80 |
| `internal/format` | 270 |
| Unit tests | ~1,200 |
| Integration tests | ~600 |
| Golden fixtures | ~400 (mostly data, not code) |
| Docs / README / scripts | ~400 |
| **Total Go code (excl. tests)** | **~6,810** |
| **Total (incl. tests/docs/fixtures)** | **~9,010** |

This is a one-shot build sized as a real engineering project — not incremental, not MVP-first. Decomposed into parallel work units per §17 it can be built by a small team working in parallel.

---

## 16. Build plan

This is a one-shot build. We ship the whole daemon end-to-end — not an MVP followed by later expansion, not a weekly rollout. The milestones below are logical groupings for dependency ordering and parallel work-unit decomposition, not phased releases. Everything ships together when Milestones 1–4 are complete; Milestone 5 is operational tuning after ship.

### Foundation (first sequential pass — NOT a stopping point)

A small amount of foundation work must land sequentially before the rest of Milestone 1 can overlap in parallel. **If you are a solo implementer, you do this first yourself, then immediately continue with the rest of Milestone 1 and all subsequent milestones in the same session — the foundation is not a handoff point, not a stopping point, and not a "progress checkpoint."** If the build is coordinated across multiple agents via `/batch`, one foundation agent lands these pieces first and then parallel workers spawn against stable interfaces. Either way, the foundation is step zero of the build, not its deliverable. Reporting "foundation done, ready for next phase" as a stopping point is a scope violation per §1 Scope Lock.

- Go module setup, `go.mod`, Makefile, CI config.
- `internal/config` package with YAML loading and validation. Unit tests.
- `internal/log` package with slog + lumberjack.
- `internal/lsp/store` package with `Publish`/`Peek`/`Wait` semantics. Unit tests with `-race`.
- `internal/lsp/client` interface definitions and `ClientManager` struct signatures. Method bodies can be stubs — the interface is what other workers need.
- `cmd/lspd` subcommand dispatch skeleton: `start`, `stop`, `ping` wired to no-op behavior so we can smoke-test the daemonization.
- `internal/daemon` skeleton: pidfile, signal handling, main loop with no-op tickers.
- `internal/mcp` skeleton: `mcp-go` server setup, one stub tool that returns hardcoded data to prove the transport works.

**Exit criteria**: `lspd start` daemonizes cleanly, `lspd ping` works, `lspd stop` exits cleanly, MCP server accepts connections and returns the stub tool result. Unit tests for config, log, store pass. Interface definitions for `ClientManager`, `Router`, `Supervisor`, `Policy` are stable enough that parallel workers can build against them without churn.

### Milestone 1: Tier-1 diagnostic pipeline

Diagnostics flow end-to-end for `.ts` and `.py` files through the MCP `getIdeDiagnostics` path. This is the foundational pipeline the whole project rides on.

Parallel work units (see §17):
- Full `internal/lsp/client` implementation (handshake, notify, requests, docs).
- `internal/lsp/router` with project root detection.
- `internal/lsp/supervisor` with crash recovery and restart logic.
- `internal/mcp/tools/compat` — `getIdeDiagnostics` handler plus the three compat stubs (`openDiff`, `closeDiff`, `openFile`).
- `internal/policy` — dedup, volume caps, severity filter, source allow/deny. Golden tests.
- TypeScript language integration test against real `typescript-language-server`.
- Python language integration test against real `pyright-langserver`.
- `cmd/lspd status` subcommand.
- Launcher wrapper shell script.

**Exit criteria**: running `droid` via the launcher wrapper on a TS or Python project with a known broken import produces a `<system-reminder>` in the agent's next turn with real LSP diagnostics. Policy golden tests pass. Unit tests pass with `-race`.

### Milestone 2: Language coverage and the Read gap

Extend diagnostic coverage to Go, Rust, C/C++ and close the Read-time gap via the socket + hook path.

Parallel work units:
- Go, Rust, C/C++ language integration tests and config entries.
- `cmd/lsp-read-hook` thin client binary.
- `internal/socket` server with `drain`, `peek`, `forget`, `status`, `ping`, `reload` ops.
- PostToolUse Read hook wiring in example `settings.json`.
- `internal/watcher` fsnotify watcher for out-of-band file changes.
- SessionStart and SessionEnd hook scripts.

**Exit criteria**: reading a file in Droid triggers the Read hook and a `<system-reminder>` with current diagnostics appears in the same agent turn. All five languages (ts/py/go/rust/cpp) work end-to-end. External editor changes are reflected in the next diagnostic query.

### Milestone 3: Semantic navigation tools (read-only tier 2)

The model-facing semantic query tools. These reshape how Droid navigates code — the model starts reaching for semantic queries over `Grep` for refactor and impact-analysis tasks.

Each tool is an independent work unit in `internal/mcp/tools/nav/`:

- `lspDefinition`
- `lspReferences`
- `lspHover`
- `lspWorkspaceSymbol`
- `lspDocumentSymbol`
- `lspCodeActions`

Plus: `internal/mcp/descriptions` with carefully-written tool descriptions; navigation integration tests; code-action attachment to diagnostics in the policy layer so quick-fix previews ride along with surfaced errors.

**Exit criteria**: all six read-only tools pass the MCP contract test. A real Droid smoke run shows the model reaching for `lspReferences` in response to "where is this function used" instead of falling back to `Grep`.

### Milestone 4: Refactor tools, observability, hardening

The refactor-shaped tier-2 tools plus the operational polish that makes the daemon production-ready.

Parallel work units:

- `lspRename` (dry-run returns the `WorkspaceEdit` for review; execution walks `changes` and applies via Droid's native `Edit`/`apply-patch` tools — `lspd` does not apply edits itself).
- `lspFormat`
- `lspCallHierarchy`
- `lspTypeHierarchy`
- `internal/metrics` Prometheus endpoint.
- `internal/config/reload.go` SIGHUP hot-reload.
- `/debug/lspd` debug endpoint.
- Structured logging cleanup pass.
- Documentation pass: README, config reference, troubleshooting guide.
- Real Droid smoke test, manual.

**Exit criteria**: `lspd reload` changes policy without restarting language servers. Debug endpoint returns useful state. All four refactor tools pass contract tests. All ten tier-2 tools are exposed and discoverable via MCP tool listing. Documentation is complete enough for a new user to install and use the daemon.

### Milestone 5: Tuning and measurement (post-ship, ongoing)

- Benchmark real Droid sessions. Measure `lspDefinition`/`lspReferences` vs `Grep` latency and choice frequency in model traces.
- Tune volume caps, severity filters, source denylists based on actual noise observed in traces.
- Tune tool descriptions based on what the model actually reaches for (descriptions are load-bearing for model behavior).
- Add per-language settings based on observed pain points.

No fixed exit criteria — this is the ongoing operational concern after shipping.

### Dependency ordering

The foundation sub-pass must land before Milestones 1–4 can spawn parallel work units. Milestone 1 starts as soon as the foundation is in. Milestones 2, 3, and 4 can start as soon as Milestone 1's MCP server, policy layer, and client manager are wired up far enough for parallel workers to build against them — they do not need to wait for Milestone 1 to be fully merged.

---

## 17. Definition of done — 39 work units (ALL must ship)

**Every work unit below must ship for the build to be considered complete.** This is the definition of done, regardless of execution mode. Do not skip units. Do not merge units into "simplified versions". Do not defer units as "future work" or "phase 2". Do not create placeholder files for units you intend to fill in later.

**Solo implementers**: execute every unit — sequentially or in any order that respects dependency ordering from §16 (foundation first, then the rest). All 39 must land in the same session. "I shipped 20 of 39" is not done.

**Coordinators using `/batch`**: decompose into parallel workers, one per unit, each in an isolated worktree. Each unit satisfies the `/batch` requirements: independently implementable, mergeable on its own without depending on sibling PRs, roughly uniform in size.

In both cases, all 39 must land before you report done.

### Milestone 1 work units

| # | Title | Primary files | Description |
|---|---|---|---|
| 1 | LSP client — handshake & lifecycle | `internal/lsp/client/manager.go`, `handshake.go`, `internal/lsp/client/manager_test.go` | Implement `ClientManager.Start/Shutdown/Wait`, `initialize`/`initialized` flow, `didChangeConfiguration`. Mock LSP server integration test. |
| 2 | LSP client — notification routing | `internal/lsp/client/notify.go`, `notify_test.go` | Reader goroutine, notification dispatch to `DiagnosticStore`, logMessage routing, progress tracking. |
| 3 | LSP client — request wrappers | `internal/lsp/client/requests.go`, `requests_test.go` | Typed Go methods for all LSP request methods we use. Each method wraps `jsonrpc2.Call` with proper param types from `go.lsp.dev/protocol`. |
| 4 | LSP client — document lifecycle | `internal/lsp/client/docs.go`, `docs_test.go` | `EnsureOpen`/`Close`/`Touch`, version tracking, TTL-based eviction. |
| 5 | LSP router & project root detection | `internal/lsp/router/router.go`, `rootdetect.go`, tests | Extension → language map, lazy spawn, project root walker using `root_markers`. |
| 6 | LSP supervisor | `internal/lsp/supervisor/supervisor.go`, tests | Crash detection, exponential backoff, restart with document re-registration, degraded state. |
| 7 | MCP server — `getIdeDiagnostics` handler | `internal/mcp/tools/compat/diagnostics.go`, tests | Full handler per §6.2.1. Integration with router, store, policy. |
| 8 | MCP server — stubs (`openDiff`, `closeDiff`, `openFile`) | `internal/mcp/tools/compat/stubs.go`, tests | Three no-op handlers that return success. |
| 9 | MCP server — StreamableHTTP wiring | `internal/mcp/server.go`, `internal/mcp/session.go` | `mcp-go` server setup, tool registration dispatch, session tracking via `X-Droid-Session-Id` header. |
| 10 | Policy layer — dedup & fingerprinting | `internal/policy/dedup.go`, tests + golden | Session state, fingerprint hash, dedupe filter. |
| 11 | Policy layer — severity, source, volume caps | `internal/policy/severity.go`, `volume.go`, tests + golden | Severity filter, source allow/deny, per-file and per-turn volume caps, sort stability. |
| 12 | TypeScript language integration test | `test/integration/ts_test.go`, `test/fixtures/ts/` | Spawns real `typescript-language-server`, asserts diagnostics for a known-broken TS file. |
| 13 | Python language integration test | `test/integration/py_test.go`, `test/fixtures/py/` | Spawns real `pyright-langserver`, asserts diagnostics for a known-broken Python file. |
| 14 | Go language integration test | `test/integration/go_test.go`, `test/fixtures/go/` | Spawns real `gopls`, asserts diagnostics for a known-broken Go file. |
| 15 | `lspd status` subcommand | `cmd/lspd/status.go`, tests | Queries socket `status` op, formats human-readable table or JSON. |
| 16 | Launcher wrapper + install script | `scripts/droid-launcher.sh`, `scripts/install.sh` | Shell wrapper that ensures lspd is up and exports `FACTORY_VSCODE_MCP_PORT` before exec. |

### Milestone 2 work units

| # | Title | Primary files | Description |
|---|---|---|---|
| 17 | Rust language integration test | `test/integration/rust_test.go` | Same as TS/Py but for rust-analyzer. Skipped in CI due to cost. |
| 18 | C/C++ language integration test | `test/integration/cpp_test.go` | Same for clangd. |
| 19 | `lsp-read-hook` binary | `cmd/lsp-read-hook/main.go`, tests | Thin client per §7.4. |
| 20 | Socket server — `drain`/`peek`/`forget` | `internal/socket/server.go`, `protocol.go`, `handlers.go`, tests | Unix socket accept loop, request dispatch, three core ops. |
| 21 | Socket server — `status`/`ping`/`reload` | Add to `internal/socket/handlers.go`, tests | Management ops on the socket. |
| 22 | fsnotify watcher | `internal/watcher/watcher.go`, tests | Watches project roots, debounces, dispatches `didChange` on out-of-band edits. |
| 23 | SessionStart + SessionEnd hook scripts | `scripts/session-start.sh`, `examples/settings.json` | Shell scripts for the fallback path and session cleanup. |

### Milestone 3 work units (read-only semantic tools)

| # | Title | Primary files | Description |
|---|---|---|---|
| 24 | `lspDefinition` tool | `internal/mcp/tools/nav/definition.go`, tests | LSP definition request wrapper, location conversion with source-line preview. |
| 25 | `lspReferences` tool | `internal/mcp/tools/nav/references.go`, tests | References with by-file grouping and per-call volume caps. |
| 26 | `lspHover` tool | `internal/mcp/tools/nav/hover.go`, tests | Hover content extraction, markdown cleanup. |
| 27 | `lspWorkspaceSymbol` tool | `internal/mcp/tools/nav/workspace_symbol.go`, tests | Fuzzy symbol search with volume cap. |
| 28 | `lspDocumentSymbol` tool | `internal/mcp/tools/nav/document_symbol.go`, tests | File outline with hierarchical structure. |
| 29 | `lspCodeActions` tool | `internal/mcp/tools/nav/code_actions.go`, tests | List quick-fixes at a position. Returns full `WorkspaceEdit` for each action so the model can apply via Droid's native `Edit`/`apply-patch` tools. `lspd` never applies edits itself. |
| 30 | MCP tool descriptions | `internal/mcp/descriptions/descriptions.go` | All tool description strings, carefully written per §6.4. Descriptions are load-bearing — they teach the model when to pick which tool. |
| 31 | Code-action attachment in policy | `internal/policy/code_actions.go`, tests | Parallel LSP `textDocument/codeAction` queries per surfaced diagnostic, attach one-line quick-fix preview to the `<system-reminder>` output. |

### Milestone 4 work units (refactor tools + observability)

| # | Title | Primary files | Description |
|---|---|---|---|
| 32 | `lspRename` tool | `internal/mcp/tools/nav/rename.go`, tests | Dry-run returns `WorkspaceEdit`; execution walks `changes` and applies via Droid's native Edit tool. |
| 33 | `lspFormat` tool | `internal/mcp/tools/nav/format.go`, tests | Document and range formatting. |
| 34 | `lspCallHierarchy` tool | `internal/mcp/tools/nav/call_hierarchy.go`, tests | Prepare + incoming/outgoing calls. |
| 35 | `lspTypeHierarchy` tool | `internal/mcp/tools/nav/type_hierarchy.go`, tests | Prepare + super/subtypes. |
| 36 | Prometheus metrics | `internal/metrics/metrics.go`, tests | Registry, all exported metrics, HTTP handler. |
| 37 | Config hot-reload via SIGHUP | `internal/config/reload.go`, tests | Diff config, apply changes to running managers without dropping connections. |
| 38 | Debug endpoint at `/debug/lspd` | Add to `internal/metrics`, tests | JSON dump of internal state, gated behind config. |
| 39 | README + config reference + troubleshooting | `README.md`, `docs/config.md`, `docs/troubleshooting.md` | User-facing documentation. |

### Coordinator notes

- **Foundation must land first.** The foundation sub-pass in §16 is sequential and provides stable interfaces all other units build against. No parallel work starts until it's merged.
- **Interface stubs before parallelism.** The foundation creates stubs for `ClientManager`, `DiagnosticStore`, `Router`, `Policy`, `ConfigService`. All Milestone 1 workers depend on these interfaces and may need to fill in stubbed methods.
- **Integration tests depend on the corresponding client manager code.** Units 12/13/14 (TS, Py, Go integration tests) should be spawned slightly after units 1–4 (the LSP client implementation), or include stubbed LSP managers in their scope.
- **Unit 9 (MCP server wiring) depends on units 1–11** to have real implementations behind the handlers. Spawn it last in Milestone 1 or after Milestone 1 merges.
- **e2e test recipe for every unit**: run the MCP contract test in `test/e2e/mcp_contract_test.go` which spins up lspd in-process, connects as an MCP client, and calls every registered tool against a test project. This is the smoke test that verifies Droid will work when `FACTORY_VSCODE_MCP_PORT` is pointed at it.
- **Unit test runner**: `go test -race ./...` at the repo root.
- **Integration test runner**: `go test -race -tags integration ./test/integration/...` (requires language servers installed on the worker machine; mark the worker as unable to run integration tests if they're not).
- **PR title convention**: `lspd: m<milestone>/<unit>: <title>` — e.g., `lspd: m1/7: MCP getIdeDiagnostics handler`.

---

## 18. Verification steps before build

These are cheap, high-value checks to run before the foundation sub-pass starts. Each answers a question whose answer could reshape the design.

### 18.1 `IdeContextManager` env var timing

**Question**: Does `IdeContextManager` read `FACTORY_VSCODE_MCP_PORT` only at construction time (early in Droid startup), or does it re-check later?

**Why it matters**: If construction-time only, the SessionStart hook path (§11.2) can't inject the port via `CLAUDE_ENV_FILE`. Only the launcher wrapper works. We should know this before documenting the fallback path as viable.

**How to verify**: Read `src/services/IdeContextManager.ts` lines 60–150 to find where `process.env.FACTORY_VSCODE_MCP_PORT` is read. If it's in a constructor or `init()` called once, it's construction-time. If it's in a connect/reconnect loop, later reads work. Run Droid with `--debug` under both conditions (wrapper and SessionStart hook) and check for "Connected to VS Code IDE" in the logs.

### 18.2 MCP session stickiness

**Question**: Does Droid's `VSCodeIdeClient` MCP client use sticky sessions (one session ID across all calls), or is every `callTool` a fresh MCP session?

**Why it matters**: This determines whether we can track per-session dedup state by MCP session header (easy) or need the caller to pass `session_id` in every tool call (requires Droid to include it, which it doesn't by default).

**How to verify**: Read `VSCodeIdeClient.ts` constructor and `callTool` method; inspect the `StreamableHTTPClientTransport` configuration from the MCP SDK to see if it sets a persistent session ID. If it does, we key dedup by session. If not, we can extract `session_id` from the `transcript_path` query param or fall back to process-level state.

### 18.3 Exact `Diagnostic` shape Droid parses

**Question**: What fields on the `IdeDiagnostic` object does Droid actually read, beyond `severity`/`message`/`source`/`range.start.line`?

**Why it matters**: We want to return exactly what Droid expects, nothing more and nothing less. Extra fields are wasted bytes; missing fields cause errors.

**How to verify**: Read `formatDiagnosticsForSystemReminder` in `diagnostics-utils.ts` in full. Also check `compareDiagnostics` and `IdeDiagnostic` type import in `@/hooks/types`. Document the minimal required shape in Appendix B of this plan.

### 18.4 `openDiff` / `closeDiff` / `openFile` tolerance

**Question**: Does Droid tolerate these three stub calls returning `"ok"` strings with no real IDE behavior, or does it expect a specific response format?

**Why it matters**: If Droid checks for specific fields, our stubs need to match. If it ignores the response entirely, a simple `"ok"` is fine.

**How to verify**: Read the three call sites in `VSCodeIdeClient.ts` (lines 404, 412, 420) and any code that consumes their return value. If the return value is thrown away, stubs are fine. If it's parsed, match the expected shape.

### 18.5 `IdeDiagnosticsTool` collision

**Question**: Droid ships a model-callable `IdeDiagnosticsTool` at `src/components/tools/implementations/IdeDiagnosticsTool.tsx`. Does exposing a tier-2 `lspDiagnostics` tool collide with it?

**Why it matters**: Tool name collisions could confuse the model or cause registration errors.

**How to verify**: Read the existing tool's implementation and registration. If it's the same thing we're exposing, drop our duplicate. If it's different (e.g., it queries a different source), ensure the names don't collide.

### 18.6 Subagent hook inheritance

**Question**: Do PostToolUse hooks fire consistently for subagents (Task tool invocations), or only for the main agent?

**Why it matters**: If subagents bypass the hook pipeline, Droid's Task-tool runs will miss Read-time diagnostics. This is a real gap we should know about and either document or close.

**How to verify**: Read `src/tools/executors/client/utils/SubagentStreamProcessor.ts` line 535 (earlier grep hit showed `executeHooks` is called there). Confirm the pathway fires for PostToolUse. If it doesn't, we'd need a different mechanism for subagent coverage or accept the gap.

### 18.7 `mcp-go` StreamableHTTP compatibility

**Question**: Does `mcp-go`'s `server.NewStreamableHTTPServer` speak the same protocol version as the TypeScript SDK's `StreamableHTTPClientTransport` that Droid uses?

**Why it matters**: Both sides implement MCP spec, but protocol version skews happen. If they're incompatible, we'd need to wrap or upgrade one side.

**How to verify**: Write a 50-line Go smoke test:

```go
package smoke

import (
    "testing"
    "context"
    "github.com/mark3labs/mcp-go/server"
    "github.com/mark3labs/mcp-go/mcp"
    // ...
)

func TestDroidCompat(t *testing.T) {
    s := server.NewMCPServer("lspd-smoke", "0.0.1", server.WithToolCapabilities(true))
    s.AddTool(mcp.NewTool("getIdeDiagnostics", mcp.WithDescription("test")),
        func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            return mcp.NewToolResultText(`{"diagnostics": []}`), nil
        })

    ln, _ := net.Listen("tcp", "127.0.0.1:0")
    port := ln.Addr().(*net.TCPAddr).Port
    go server.NewStreamableHTTPServer(s).Serve(ln)

    // Start real droid with FACTORY_VSCODE_MCP_PORT=port, run a single edit,
    // assert the edit-cli logs show "Fetched IDE diagnostics" with an empty result.
}
```

Run this before the foundation sub-pass starts. If it passes, green-light the architecture. If it fails, debug the transport mismatch before writing any more code.

---

## Appendix A: Source references

All file paths are relative to `/Users/harsha/.factory/droid-source-code/`.

### Droid MCP IDE integration
- `src/services/VSCodeIdeClient.ts:1–2` — imports `Client` and `StreamableHTTPClientTransport` from `@modelcontextprotocol/sdk`
- `src/services/VSCodeIdeClient.ts:86` — `class VSCodeIdeClient`
- `src/services/VSCodeIdeClient.ts:107–158` — `connect()` method, reads `FACTORY_VSCODE_MCP_PORT`
- `src/services/VSCodeIdeClient.ts:190` — `callTool(name, args): Promise<string>`
- `src/services/VSCodeIdeClient.ts:404` — `callTool('openDiff', ...)`
- `src/services/VSCodeIdeClient.ts:412` — `callTool('closeDiff', ...)`
- `src/services/VSCodeIdeClient.ts:420` — `callTool('openFile', ...)`
- `src/services/IdeContextManager.ts:65–66` — env var detection and auto-connect
- `src/services/IdeContextManager.ts:150` — `new VSCodeIdeClient({...})`
- `src/services/IdeContextManager.ts:205` — `getIdeClient()`
- `src/services/JetBrainsIdeClient.ts` — parallel implementation for JetBrains; same MCP contract

### Diagnostic pipeline
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:38` — `fetchDiagnostics(ideClient, filePath, maxRetries, delayMs)`
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:52` — `ideClient.callTool('getIdeDiagnostics', { uri })`
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:111` — `compareDiagnostics(before, after)`
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:117` — severity filter to 0 (Error) and 1 (Warning)
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:130–134` — comparison key: `message + range.start.line + severity`
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:143` — `formatDiagnosticsForSystemReminder`
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:152–153` — `<system-reminder>` wrapper
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:160–167` — Errors section formatting
- `src/tools/executors/client/file-tools/diagnostics-utils.ts:169–177` — Warnings section formatting

### Tool executor call sites
- `src/tools/executors/client/edit-cli.ts:13–15` — imports
- `src/tools/executors/client/edit-cli.ts:104` — `diagnosticsBefore`
- `src/tools/executors/client/edit-cli.ts:144` — `diagnosticsAfter` with 1 retry, 500ms
- `src/tools/executors/client/edit-cli.ts:152` — `compareDiagnostics`
- `src/tools/executors/client/edit-cli.ts:158` — `formatDiagnosticsForSystemReminder`
- `src/tools/executors/client/create-cli.ts:25–26` — imports
- `src/tools/executors/client/create-cli.ts:191` — `diagnosticsAfter` (no before for create)
- `src/tools/executors/client/create-cli.ts:202` — system reminder
- `src/tools/executors/client/apply-patch-cli.ts:35–37` — imports
- `src/tools/executors/client/apply-patch-cli.ts:248` — before snapshot
- `src/tools/executors/client/apply-patch-cli.ts:343–358` — after snapshot, compare, format
- `src/tools/executors/client/read-cli.ts` — **no** diagnostic calls (the Read gap)

### Hook system
- `src/services/HookService.ts:1` — `import { spawn } from 'child_process'`
- `src/services/HookService.ts:15` — `async executeHooks(params)`
- `src/services/HookService.ts:232` — `executeCommand(command, input, timeout)`
- `src/services/HookService.ts:242–254` — env var injection from input fields
- `src/services/HookService.ts:256–267` — `FACTORY_PROJECT_DIR`, `DROID_PROJECT_DIR`, `CLAUDE_PROJECT_DIR`, plugin roots
- `src/services/HookService.ts:269–277` — shell selection and spawn with timeout (NOT detached)
- `src/services/HookService.ts:291–342` — close handler, JSON output parsing
- `src/services/HookService.ts:300–332` — `hookSpecificOutput` parsing
- `src/services/HookService.ts:384` — `getHookService()` singleton

- `src/services/hook-utils.ts:39` — `executeHooksWithDisplay`
- `src/services/hook-utils.ts:70–106` — matcher filter with regex support
- `src/services/hook-utils.ts:112–116` — collect all matching hook commands
- `src/services/hook-utils.ts:145` — `getHookService().executeHooks(...)`

- `src/services/SessionService.ts:331–334` — `pendingSessionStartContext`, `sessionEndHooksExecuted`
- `src/services/SessionService.ts:1342` — SessionStart on new session
- `src/services/SessionService.ts:2600` — SessionStart on resume
- `src/services/SessionService.ts:3192` — `consumePendingSessionStartContext`
- `src/services/SessionService.ts:3679–3767` — `executeSessionEndHooks`
- `src/services/SessionService.ts:3769–3906` — `executeSessionStartHooks`
- `src/services/SessionService.ts:3801` — `await getHookService().executeHooks(...)` (synchronous)
- `src/services/SessionService.ts:3812` — `CLAUDE_ENV_FILE` field
- `src/services/SessionService.ts:3818–3859` — env file parsing and application to `process.env`
- `src/services/SessionService.ts:3862–3890` — `additionalContext` collection for LLM injection
- `src/services/SessionService.ts:4072–4113` — `registerSessionEndHook()` via shutdown coordinator
- `src/services/SessionService.ts:4100–4107` — shutdown hook registration at priority `SHUTDOWN_HOOK_PRIORITY.SessionEnd`

- `src/core/ToolExecutor.ts:72` — `import { executeHooksWithDisplay }`
- `src/core/ToolExecutor.ts:651` — PreToolUse execution
- `src/core/ToolExecutor.ts:1376–1449` — PostToolUse execution block
- `src/core/ToolExecutor.ts:1385` — `await executeHooksWithDisplay(PostToolUse, ...)`
- `src/core/ToolExecutor.ts:1412–1425` — `additionalContext` → `ADD_MESSAGE` dispatch with `role: System`

### Tier-2 related existing Droid code
- `src/components/tools/implementations/IdeDiagnosticsTool.tsx` — existing model-callable diagnostic tool (collision check in §18.5)
- `src/tools/executors/client/utils/SubagentStreamProcessor.ts:535` — subagent hook execution (verification item §18.6)
- `src/app.tsx:1697` — early MCP-related hook execution

---

## Appendix B: LSP Diagnostic JSON shape

Exact shape Droid expects from `getIdeDiagnostics` responses, confirmed by field accesses in `diagnostics-utils.ts`:

```ts
// VS Code DiagnosticSeverity (numeric enum)
// 0 = Error
// 1 = Warning
// 2 = Information
// 3 = Hint

interface IdeDiagnostic {
  severity: number;               // required; Droid filters to 0,1
  message: string;                // required; used in the system reminder
  source?: string;                // optional but recommended; shown in parens
  range: {
    start: { line: number; character: number };  // required; 0-indexed
    end:   { line: number; character: number };  // required; 0-indexed
  };
  code?: string | number;         // optional; not used in formatting but useful for dedup
  relatedInformation?: any[];     // optional; currently ignored by Droid formatter
  tags?: number[];                // optional; currently ignored
}

interface GetIdeDiagnosticsResponse {
  diagnostics: IdeDiagnostic[];
}
```

The field accesses in `formatDiagnosticsForSystemReminder`:

```ts
error.range.start.line + 1    // Converts 0-indexed to 1-indexed for display
error.message                 // Required, printed directly
error.source ? ` (${error.source})` : ''  // Optional, wrapped in parens
```

And in `compareDiagnostics`:

```ts
beforeError.message === afterError.message &&
beforeError.range.start.line === afterError.range.start.line &&
beforeError.severity === afterError.severity
```

So the comparison key is `(message, startLine, severity)` — Droid dedupes on these three fields only. Our `lspd` dedup layer uses a stronger key (`uri + line + col + code + source + message`) because we want stricter dedup across multiple sessions and don't want unrelated files with the same message at the same line number to collide.

---

## Appendix C: `mcp-go` API sketch

Based on `droid-lsp/mcp-go/`.

### Server setup

```go
import (
    "github.com/mark3labs/mcp-go/server"
    "github.com/mark3labs/mcp-go/mcp"
)

s := server.NewMCPServer(
    "lspd",                 // server name
    "0.1.0",                // version
    server.WithToolCapabilities(true),
    server.WithLogging(),
)
```

### Tool registration

```go
tool := mcp.NewTool("getIdeDiagnostics",
    mcp.WithDescription("Fetch LSP diagnostics for a file URI."),
    mcp.WithString("uri",
        mcp.Required(),
        mcp.Description("File URI (file:///...)"),
    ),
)

s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    uri := req.GetString("uri", "")
    // ... handler body ...
    return mcp.NewToolResultJSON(response), nil
})
```

### Typed tools (for stronger typing)

```go
type GetIdeDiagnosticsArgs struct {
    URI string `json:"uri" validate:"required"`
}

s.AddTypedTool(mcp.NewTool("getIdeDiagnostics", ...),
    func(ctx context.Context, req mcp.CallToolRequest, args GetIdeDiagnosticsArgs) (*mcp.CallToolResult, error) {
        // args.URI is already parsed and validated
        return ..., nil
    })
```

### StreamableHTTP server

```go
httpServer := server.NewStreamableHTTPServer(s)
ln, err := net.Listen("tcp", "127.0.0.1:0")
if err != nil { return err }
port := ln.Addr().(*net.TCPAddr).Port
go httpServer.Serve(ln)  // or Start(addr) which binds internally
```

### Session extraction

```go
func getSessionID(ctx context.Context) string {
    // mcp-go exposes the session ID from the HTTP request headers via ctx
    s, ok := server.SessionIDFromContext(ctx)
    if !ok {
        return ""
    }
    return s
}
```

See `droid-lsp/mcp-go/examples/typed_tools` for a full worked example.

---

## Appendix D: `go.lsp.dev` API sketch

Based on the `go.lsp.dev/protocol` and `go.lsp.dev/jsonrpc2` packages (the same libraries `gopls` uses).

### Connection setup

```go
import (
    "context"
    "os/exec"
    "go.lsp.dev/jsonrpc2"
    "go.lsp.dev/protocol"
)

cmd := exec.Command("typescript-language-server", "--stdio")
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Start()

stream := jsonrpc2.NewStream(readWriteCloser{stdout, stdin})
conn := jsonrpc2.NewConn(stream)
```

### Initialize

```go
var initResult protocol.InitializeResult
err := conn.Call(ctx, protocol.MethodInitialize, &protocol.InitializeParams{
    ProcessID: int32(os.Getpid()),
    RootURI:   protocol.DocumentURI("file://" + projectRoot),
    Capabilities: protocol.ClientCapabilities{
        TextDocument: &protocol.TextDocumentClientCapabilities{
            PublishDiagnostics: &protocol.PublishDiagnosticsClientCapabilities{
                RelatedInformation: true,
            },
            Definition: &protocol.DefinitionTextDocumentClientCapabilities{
                LinkSupport: true,
            },
            // ... more capabilities ...
        },
    },
    WorkspaceFolders: []protocol.WorkspaceFolder{
        {URI: "file://" + projectRoot, Name: filepath.Base(projectRoot)},
    },
}, &initResult)

// Send 'initialized' notification
conn.Notify(ctx, protocol.MethodInitialized, &protocol.InitializedParams{})
```

### Document notifications

```go
conn.Notify(ctx, protocol.MethodTextDocumentDidOpen, &protocol.DidOpenTextDocumentParams{
    TextDocument: protocol.TextDocumentItem{
        URI:        protocol.DocumentURI("file://" + path),
        LanguageID: "typescript",
        Version:    1,
        Text:       content,
    },
})

conn.Notify(ctx, protocol.MethodTextDocumentDidChange, &protocol.DidChangeTextDocumentParams{
    TextDocument: protocol.VersionedTextDocumentIdentifier{
        TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: ...},
        Version:                2,
    },
    ContentChanges: []protocol.TextDocumentContentChangeEvent{
        {Text: newContent},  // full-document sync
    },
})
```

### Request wrappers

```go
var locations []protocol.Location
err := conn.Call(ctx, protocol.MethodTextDocumentDefinition, &protocol.DefinitionParams{
    TextDocumentPositionParams: protocol.TextDocumentPositionParams{
        TextDocument: protocol.TextDocumentIdentifier{URI: uri},
        Position:     protocol.Position{Line: line, Character: col},
    },
}, &locations)
```

### Notification handler (for publishDiagnostics)

```go
conn.SetHandler(jsonrpc2.HandlerFunc(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
    switch req.Method() {
    case protocol.MethodTextDocumentPublishDiagnostics:
        var params protocol.PublishDiagnosticsParams
        if err := json.Unmarshal(req.Params(), &params); err != nil {
            return err
        }
        store.Publish(params.URI, params.Version, params.Diagnostics, "typescript")
        return reply(ctx, nil, nil)
    case "window/logMessage":
        // ...
    }
    return reply(ctx, nil, jsonrpc2.ErrMethodNotFound)
}))
```

---

## End of plan

This plan is comprehensive and self-contained. To execute:

**If you are a solo implementer:**

1. Read `SOLO_IMPL_CONTRACT.md` at the repo root. It is your scope lock.
2. Verify items §18.1–18.7.
3. Land the foundation pieces from §16 in one sequential pass so interfaces are stable.
4. Immediately continue through every work unit in §17 (all 39) in the same session. Foundation is not a stopping point. Milestone 1 completion is not a stopping point. Only stop when every work unit has shipped and every success criterion in §1.3 is satisfied.
5. Run the verification commands from the builder prompt / §14 before declaring done.

**If you are a `/batch` coordinator:**

1. Verify items §18.1–18.7.
2. Spawn one foundation agent to land the sequential pieces from §16.
3. On foundation merge, spawn Milestone 1 work units in parallel.
4. As Milestone 1's MCP server, policy layer, and client manager wire up, spawn Milestone 2, 3, and 4 work units in parallel — they do not need to wait for Milestone 1 to fully merge.
5. Ship when all 39 work units from §17 have landed. Enter Milestone 5 tuning based on real usage after ship.

**This is a one-shot build.** No MVP release, no weekly rollout, no "phase 1 first and we'll see about the rest." The whole daemon ships together when it's done. Partial deliveries are scope violations per §1 Scope Lock.
